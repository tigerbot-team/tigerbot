package headingholder

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder/angle"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/bno08x"
)

const motorFullRange = 0x1000

func NewYawRateAndThrottle(motors RawControl) *YawRateAndThrottle {
	return &YawRateAndThrottle{
		Motors: motors,
	}
}

type RawControl interface {
	SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error
}

type YawRateAndThrottle struct {
	Motors RawControl

	controlLock                    sync.Mutex
	yawRate, throttle, translation float64
	currentHeading                 angle.PlusMinus180
}

func (h *YawRateAndThrottle) SetYawAndThrottle(yawRate, throttle, translation float64) {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()

	h.yawRate = yawRate
	h.throttle = throttle
	h.translation = translation
}

func (h *YawRateAndThrottle) CurrentHeading() angle.PlusMinus180 {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()

	return h.currentHeading
}

func (h *YawRateAndThrottle) Loop(cxt context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	defer fmt.Println("Heading holder loop exited")

	m, lastIMUReport, err := openIMU(cxt)
	if err != nil {
		return
	}

	var headingEstimate angle.PlusMinus180
	var targetHeading = lastIMUReport.RobotYaw()
	var filteredTranslation, filteredThrottle float64
	var motorRotationSpeed float64
	var lastHeadingError float64
	var iHeadingError float64

	const (
		kp                     = 0.02
		ki                     = 0.03
		kd                     = -0.00007
		maxIntegral            = .1
		maxMotorSpeed          = 2.0
		maxThrottleDeltaPerSec = 2
	)
	maxThrottleDelta := maxThrottleDeltaPerSec * bno08x.ReportInterval.Seconds()
	maxTranslationDelta := maxThrottleDelta
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
		headingEstimate = imuReport.RobotYaw()

		// Grab the current control values.
		h.controlLock.Lock()
		targetYawRate := h.yawRate
		targetThrottle := h.throttle
		targetTranslation := h.translation
		h.currentHeading = headingEstimate
		h.controlLock.Unlock()

		// Update our target heading accordingly.
		loopTimeSecs := loopTime.Seconds()

		// Avoid letting the yaw lead the heading too much.
		const yawRateFactor = 300
		targetHeading = targetHeading.AddFloat(loopTimeSecs * targetYawRate * yawRateFactor)
		leadDegrees := targetHeading.Sub(headingEstimate)

		const maxLeadDegrees = 20.0
		if leadDegrees.Float() > maxLeadDegrees {
			targetHeading = headingEstimate.AddFloat(maxLeadDegrees)
		} else if leadDegrees.Float() < -maxLeadDegrees {
			targetHeading = headingEstimate.SubFloat(maxLeadDegrees)
		}

		// Calculate the error/derivative/integral.
		headingError := targetHeading.Sub(headingEstimate).Float()
		dHeadingError := (headingError - lastHeadingError) / loopTimeSecs
		iHeadingError += headingError * loopTimeSecs
		if targetYawRate != 0 {
			iHeadingError = 0
		}
		if iHeadingError > maxIntegral {
			iHeadingError = maxIntegral
		} else if iHeadingError < -maxIntegral {
			iHeadingError = -maxIntegral
		}

		// Calculate the correction to apply.
		rotationCorrection := kp*headingError + ki*iHeadingError + kd*dHeadingError

		// Add the correction to the current speed.  We want 0 correction to mean "hold the same motor speed".
		motorRotationSpeed = rotationCorrection
		if motorRotationSpeed > maxMotorSpeed {
			motorRotationSpeed = maxMotorSpeed
		} else if motorRotationSpeed < -maxMotorSpeed {
			motorRotationSpeed = -maxMotorSpeed
		}

		fmt.Printf("HH: %v Heading: %.1f Target: %.1f Error: %.1f Int: %.1f D: %.1f -> %.1f\n",
			loopTime, headingEstimate, targetHeading, headingError, iHeadingError, dHeadingError, motorRotationSpeed)

		if targetThrottle > filteredThrottle+maxThrottleDelta {
			filteredThrottle += maxThrottleDelta
		} else if targetThrottle < filteredThrottle-maxThrottleDelta {
			filteredThrottle -= maxThrottleDelta
		} else {
			filteredThrottle = targetThrottle
		}
		if targetTranslation > filteredTranslation+maxTranslationDelta {
			filteredTranslation += maxTranslationDelta
		} else if targetTranslation < filteredTranslation-maxTranslationDelta {
			filteredTranslation -= maxTranslationDelta
		} else {
			filteredTranslation = targetTranslation
		}

		// Map the values to speeds for each motor.  Motor rotation direction:
		// positive = anti-clockwise.
		frontLeft := filteredThrottle - motorRotationSpeed - filteredTranslation
		backLeft := filteredThrottle - motorRotationSpeed + filteredTranslation

		frontRight := -filteredThrottle - motorRotationSpeed - filteredTranslation
		backRight := -filteredThrottle - motorRotationSpeed + filteredTranslation

		m := max(frontLeft, frontRight, backLeft, backRight)
		scale := 1.0
		if m > 1 {
			scale = 1.0 / m
		}

		fl := scaleMotorOutput(frontLeft*scale, motorFullRange)
		fr := scaleMotorOutput(frontRight*scale, motorFullRange)
		bl := scaleMotorOutput(backLeft*scale, motorFullRange)
		br := scaleMotorOutput(backRight*scale, motorFullRange)

		if err := h.Motors.SetMotorSpeeds(fl, fr, bl, br); err != nil {
			fmt.Println("Failed to set motor speeds:", err)
		}

		lastHeadingError = headingError
		lastIMUReport = imuReport
	}
	if err := h.Motors.SetMotorSpeeds(0, 0, 0, 0); err != nil {
		fmt.Println("Failed to set motor speeds:", err)
	}
}

func openIMU(cxt context.Context) (*bno08x.BNO08X, bno08x.IMUReport, error) {
	m := bno08x.New()
	go m.LoopReadingReports(cxt)

	lastPrint := time.Now()
	var lastIMUReport bno08x.IMUReport
	for {
		if cxt.Err() != nil {
			fmt.Println("Context finished.")
			return nil, bno08x.IMUReport{}, cxt.Err()
		}
		if lastIMUReport = m.CurrentReport(); !lastIMUReport.Time.IsZero() {
			break
		}
		if time.Since(lastPrint) > time.Second {
			fmt.Println("Waiting for first reading from IMU...")
			lastPrint = time.Now()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return m, lastIMUReport, nil
}

func scaleMotorOutput(value, multiplier float64) int16 {
	multiplied := value * multiplier
	if multiplied <= math.MinInt16 {
		return math.MinInt16
	}
	if multiplied >= math.MaxInt16 {
		return math.MaxInt16
	}
	return int16(multiplied)
}
