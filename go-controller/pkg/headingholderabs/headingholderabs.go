package headingholderabs

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/imu"
)

type RawControl interface {
	SetMotorSpeeds(left, right int8)
}

func New(prop RawControl) *HeadingHolder {
	hh := &HeadingHolder{
		Motors: prop,
	}
	hh.onNewReading = sync.NewCond(&hh.controlLock)
	return hh
}

type HeadingHolder struct {
	Motors RawControl

	onNewReading *sync.Cond

	controlLock sync.Mutex
	controls
}

type controls struct {
	targetHeading  float64
	currentHeading float64
	throttle       float64
}

func (h *HeadingHolder) SetHeading(desiredHeaading float64) {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()
	h.targetHeading = desiredHeaading
}

func (h *HeadingHolder) AddHeadingDelta(delta float64) {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()
	h.targetHeading += delta
}

func (h *HeadingHolder) SetThrottle(throttle float64) {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()
	h.throttle = throttle
}

func (h *HeadingHolder) CurrentHeading() float64 {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()

	return h.currentHeading
}

func (h *HeadingHolder) TargetHeading() float64 {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()

	return h.targetHeading
}

func (h *HeadingHolder) Wait(ctx context.Context) (float64, error) {
	lastAngleError := 0.0
	numIterationsAroundZero := 0
	for {
		h.controlLock.Lock()
		if ctx.Err() != nil {
			h.controlLock.Unlock()
			return 0, ctx.Err()
		}
		h.onNewReading.Wait()
		tH := h.targetHeading
		cH := h.currentHeading
		h.controlLock.Unlock()

		angleError := tH - cH
		fmt.Printf("HH: wait angle error: %.2f last: %.2f\n", angleError, lastAngleError)
		const (
			angleThresh = 1
			iterThresh  = 7
		)
		var deltaMagnitude = 5.0
		if lastAngleError != 0 {
			delta := lastAngleError - angleError
			deltaMagnitude = math.Abs(float64(delta))
		}
		if angleError > -angleThresh && angleError < angleThresh ||
			angleError > 0 && lastAngleError < 0 ||
			angleError < 0 && lastAngleError > 0 ||
			deltaMagnitude < 0.1 {
			numIterationsAroundZero++
		}
		if numIterationsAroundZero > iterThresh {
			return angleError, nil
		}
		lastAngleError = angleError
	}
}

func (h *HeadingHolder) Loop(cxt context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	defer fmt.Println("Heading holder loop exited")
	defer func() {
		h.controlLock.Lock()
		h.onNewReading.Broadcast()
		h.controlLock.Unlock()
	}()

	m, err := imu.NewSPI("/dev/spidev0.1")
	if err != nil {
		fmt.Println("Failed to open IMU", err)
		panic("Failed to open IMU")
	}

	err = m.Configure()
	if err != nil {
		fmt.Println("Failed to open IMU", err)
		panic("Failed to open IMU")
	}
	err = m.Calibrate()

	if err != nil {
		fmt.Println("Failed to open IMU", err)
		panic("Failed to open IMU")
	}
	m.ResetFIFO()

	const imuDT = 1 * time.Millisecond
	const targetLoopDT = 20 * time.Millisecond

	ticker := time.NewTicker(targetLoopDT)
	defer ticker.Stop()

	var headingEstimate float64
	var filteredThrottle float64
	var motorRotationSpeed float64
	var lastHeadingError float64
	var iHeadingError float64

	const (
		kp                     = 0.01
		ki                     = 0.03
		kd                     = 0.0001
		maxIntegral            = 0.3
		maxD                   = 100
		maxRotationThrottle    = 0.1
		maxThrottleDeltaPerSec = 0.2
	)
	maxThrottleDelta := maxThrottleDeltaPerSec * targetLoopDT.Seconds()
	var lastLoopStart = time.Now()

	for cxt.Err() == nil {
		<-ticker.C
		now := time.Now()
		loopTime := now.Sub(lastLoopStart)
		lastLoopStart = now

		// Integrate the output from the IMU to get our heading estimate.
		yawReadings := m.ReadFIFO()

		for _, yaw := range yawReadings {
			yawDegreesPerSec := float64(yaw) * m.DegreesPerLSB()
			headingEstimate -= imuDT.Seconds() * yawDegreesPerSec
		}

		// Grab the current control values.
		h.controlLock.Lock()
		controls := h.controls
		h.currentHeading = headingEstimate
		h.onNewReading.Broadcast()
		h.controlLock.Unlock()

		// Update our target heading accordingly.
		loopTimeSecs := loopTime.Seconds()
		targetHeading := controls.targetHeading

		// Calculate the error/derivative/integral.
		headingError := targetHeading - headingEstimate
		dHeadingError := (headingError - lastHeadingError) / loopTimeSecs
		if dHeadingError > maxD {
			dHeadingError = maxD
		} else if dHeadingError < -maxD {
			dHeadingError = -maxD
		}
		iHeadingError += headingError * loopTimeSecs
		if iHeadingError > maxIntegral {
			iHeadingError = maxIntegral
		} else if iHeadingError < -maxIntegral {
			iHeadingError = -maxIntegral
		}

		// Calculate the correction to apply.
		rotationCorrection := kp*headingError + ki*iHeadingError + kd*dHeadingError

		// Add the correction to the current speed.  We want 0 correction to mean "hold the same motor speed".
		motorRotationSpeed = rotationCorrection
		if motorRotationSpeed > maxRotationThrottle {
			motorRotationSpeed = maxRotationThrottle
		} else if motorRotationSpeed < -maxRotationThrottle {
			motorRotationSpeed = -maxRotationThrottle
		}

		fmt.Printf("HH: %v %d Heading: %.1f Target: %.1f Error: %.1f Int: %.1f D: %.1f -> %.3f\n",
			loopTime, len(yawReadings), headingEstimate, targetHeading, headingError, iHeadingError, dHeadingError, motorRotationSpeed)

		targetThrottle := controls.throttle
		if math.Abs(targetThrottle) < 0.4 {
			filteredThrottle = targetThrottle
		} else if targetThrottle > filteredThrottle+maxThrottleDelta {
			filteredThrottle += maxThrottleDelta
		} else if targetThrottle < filteredThrottle-maxThrottleDelta {
			filteredThrottle -= maxThrottleDelta
		} else {
			filteredThrottle = targetThrottle
		}

		// Map the values to speeds for each motor.
		frontLeft := filteredThrottle + motorRotationSpeed
		frontRight := filteredThrottle - motorRotationSpeed

		m := math.Max(frontLeft, frontRight)
		scale := 1.0
		if m > 1 {
			scale = 1.0 / m
		}

		l := scaleAndClamp(frontLeft*scale, 127)
		r := scaleAndClamp(frontRight*scale, 127)

		h.Motors.SetMotorSpeeds(l, r)

		lastHeadingError = headingError
	}
	h.Motors.SetMotorSpeeds(0, 0)
}

func scaleAndClamp(value, multiplier float64) int8 {
	multiplied := value * multiplier
	if multiplied <= math.MinInt8 {
		return math.MinInt8
	}
	if multiplied >= math.MaxInt8 {
		return math.MaxInt8
	}
	return int8(multiplied)
}
