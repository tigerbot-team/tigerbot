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
	return &HeadingHolder{
		Motors: prop,
	}
}

type HeadingHolder struct {
	Motors RawControl

	controlLock sync.Mutex
	controls
}

type controls struct {
	desiredHeading float64
	currentHeading float64
	throttle       float64
}

func (y *HeadingHolder) SetHeading(desiredHeaading float64) {
	y.controlLock.Lock()
	defer y.controlLock.Unlock()
	y.desiredHeading = desiredHeaading
}

func (y *HeadingHolder) AddHeadingDelta(delta float64) {
	y.controlLock.Lock()
	defer y.controlLock.Unlock()
	y.desiredHeading += delta
}

func (y *HeadingHolder) SetThrottle(throttle float64) {
	y.controlLock.Lock()
	defer y.controlLock.Unlock()
	y.throttle = throttle
}

func (y *HeadingHolder) CurrentHeading() float64 {
	y.controlLock.Lock()
	defer y.controlLock.Unlock()

	return y.currentHeading
}

func (y *HeadingHolder) Loop(cxt context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	defer fmt.Println("Heading holder loop exited")

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

	const imuDT = 10 * time.Millisecond
	const targetLoopDT = 20 * time.Millisecond

	ticker := time.NewTicker(targetLoopDT)
	defer ticker.Stop()

	var headingEstimate float64
	var filteredThrottle float64
	var motorRotationSpeed float64
	var lastHeadingError float64
	var iHeadingError float64

	const (
		kp                        = 0.023
		ki                        = 0.0
		kd                        = -0.00007
		maxIntegral               = 1
		maxRotationThrottle       = 0.3
		maxTranslationDeltaPerSec = 1
		maxThrottleDeltaPerSec    = 0.5
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
		y.controlLock.Lock()
		controls := y.controls
		y.currentHeading = headingEstimate
		y.controlLock.Unlock()

		// Update our target heading accordingly.
		loopTimeSecs := loopTime.Seconds()
		targetHeading := controls.desiredHeading

		const maxLeadDegrees = 20
		if targetHeading > headingEstimate+maxLeadDegrees {
			targetHeading = headingEstimate + maxLeadDegrees
		} else if targetHeading < headingEstimate-maxLeadDegrees {
			targetHeading = headingEstimate - maxLeadDegrees
		}

		// Calculate the error/derivative/integral.
		headingError := targetHeading - headingEstimate
		dHeadingError := (headingError - lastHeadingError) / loopTimeSecs
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

		fmt.Printf("HH: %v Heading: %.1f Target: %.1f Error: %.1f Int: %.1f D: %.1f -> %.1f\n",
			loopTime, headingEstimate, targetHeading, headingError, iHeadingError, dHeadingError, motorRotationSpeed)

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

		y.Motors.SetMotorSpeeds(l, r)

		lastHeadingError = headingError
	}
	y.Motors.SetMotorSpeeds(0, 0)
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
