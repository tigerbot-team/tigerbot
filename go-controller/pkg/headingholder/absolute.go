package headingholder

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/bno08x"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder/angle"
)

func NewAbsolute(motors RawControl) *Absolute {
	hh := &Absolute{
		Motors: motors,
	}
	hh.onNewReading = sync.NewCond(&hh.controlLock)
	return hh
}

type Absolute struct {
	Motors RawControl

	onNewReading *sync.Cond

	controlLock sync.Mutex
	controls
}

type controls struct {
	targetHeading  angle.PlusMinus180
	currentHeading angle.PlusMinus180
	throttle       float64
}

func (h *Absolute) SetHeading(desiredHeaading float64) {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()

	h.targetHeading = angle.FromFloat(desiredHeaading)
}

func (h *Absolute) AddHeadingDelta(delta float64) {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()
	h.targetHeading = h.targetHeading.AddFloat(delta)
}

func (h *Absolute) SetThrottle(throttle float64) {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()
	h.throttle = throttle
}

func (h *Absolute) CurrentHeading() angle.PlusMinus180 {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()

	return h.currentHeading
}

func (h *Absolute) TargetHeading() angle.PlusMinus180 {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()

	return h.targetHeading
}

// Wait waits for the in-progress rotation to complete.
func (h *Absolute) Wait(ctx context.Context) (float64, error) {
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

		angleError := tH.Sub(cH).Float()
		fmt.Printf("HH: wait angle error: %.2f last: %.2f\n", angleError, lastAngleError)
		const (
			angleThresh = 1
			iterThresh  = 7
		)
		var deltaMagnitude = 5.0
		if lastAngleError != 0 {
			delta := lastAngleError - angleError
			deltaMagnitude = math.Abs(delta)
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

func (h *Absolute) Loop(cxt context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	defer fmt.Println("Heading holder loop exited")
	defer func() {
		h.controlLock.Lock()
		h.onNewReading.Broadcast()
		h.controlLock.Unlock()
	}()

	m, lastIMUReport, err := openIMU(cxt)
	if err != nil {
		return
	}

	initialHeading := lastIMUReport.RobotYaw()
	var headingEstimate angle.PlusMinus180
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
		maxRotationThrottle    = 0.3
		maxThrottleDeltaPerSec = 1
	)
	maxThrottleDelta := maxThrottleDeltaPerSec * bno08x.ReportInterval.Seconds()
	var lastLoopStart = time.Now()

	defer func() {
		if err := h.Motors.SetMotorSpeeds(0, 0, 0, 0); err != nil {
			fmt.Println("Failed to set motor speeds:", err)
		}
	}()
	for cxt.Err() == nil {
		// This should pop every 10ms
		imuReport := m.WaitForReportAfter(lastIMUReport.Time)

		now := time.Now()
		loopTime := now.Sub(lastLoopStart)
		lastLoopStart = now

		// We use an angle.PlusMinus180 to make sure we do our modulo arithmetic
		// correctly...
		headingEstimate = imuReport.RobotYaw().Sub(initialHeading)

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
		headingError := targetHeading.Sub(headingEstimate).Float()
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

		fmt.Printf("HH: %v Heading: %.1f Target: %.1f Error: %.1f Int: %.1f D: %.1f -> %.3f\n",
			loopTime, headingEstimate, targetHeading, headingError, iHeadingError, dHeadingError, motorRotationSpeed)

		targetThrottle := controls.throttle
		if targetThrottle > filteredThrottle+maxThrottleDelta {
			filteredThrottle += maxThrottleDelta
			fmt.Printf("HH capping throttle delta, target: %.2f capped: %.2f\n", targetThrottle, filteredThrottle)
		} else if targetThrottle < filteredThrottle-maxThrottleDelta {
			filteredThrottle -= maxThrottleDelta
			fmt.Printf("HH capping throttle delta, target: %.2f capped: %.2f\n", targetThrottle, filteredThrottle)
		} else {
			filteredThrottle = targetThrottle
		}

		// Map the values to speeds for each motor.
		left := filteredThrottle - motorRotationSpeed
		right := -filteredThrottle - motorRotationSpeed

		m := math.Max(left, right)
		scale := 1.0
		if m > 1 {
			scale = 1.0 / m
		}

		l := scaleMotorOutput(left*scale, motorFullRange)
		r := scaleMotorOutput(right*scale, motorFullRange)

		if err := h.Motors.SetMotorSpeeds(l, r, l, r); err != nil {
			fmt.Println("Failed to set motor speeds:", err)
		}

		lastHeadingError = headingError
	}
}
