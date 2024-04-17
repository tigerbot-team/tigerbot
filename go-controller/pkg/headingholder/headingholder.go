package headingholder

import (
	"context"
	"fmt"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/chassis"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/picobldc"
	"math"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder/angle"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/bno08x"
)

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

	controlLock sync.Mutex
	relativeControls
	currentHeading angle.PlusMinus180
}

type relativeControls struct {
	yawRateDegreesPerS, throttleMMPerS, translationMMPerS float64
}

func (h *YawRateAndThrottle) SetYawAndThrottle(yawRate, throttle, translation float64) {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()

	h.yawRateDegreesPerS = yawRate * 500
	h.throttleMMPerS = throttle * 800
	h.translationMMPerS = translation * 800
}

func (h *YawRateAndThrottle) CurrentHeading() angle.PlusMinus180 {
	h.controlLock.Lock()
	defer h.controlLock.Unlock()

	return h.currentHeading
}

func (h *YawRateAndThrottle) Loop(cxt context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	defer fmt.Println("Heading holder loop exited")

	m, imuReport, err := openIMU(cxt)
	if err != nil {
		return
	}

	initialHeading := imuReport.RobotYaw()
	targetHeading := angle.FromFloat(0)
	var headingEstimate angle.PlusMinus180
	var filteredThrottle float64
	var filteredTranslation float64
	var lastHeadingError float64
	var iHeadingError float64

	const (
		maxRotationMMPerS      = 2000
		maxThrottleDeltaPerSec = 5000
		maxRPS                 = 10
	)

	var lastLoopStart = time.Now()

	defer func() {
		if err := h.Motors.SetMotorSpeeds(0, 0, 0, 0); err != nil {
			fmt.Println("Failed to set motor speeds:", err)
		}
	}()
	lastPrint := time.Now()
	for cxt.Err() == nil {
		// This should pop every 10ms
		lastIMUReportTime := imuReport.Time
		imuReport = m.WaitForReportAfter(lastIMUReportTime)

		now := time.Now()
		loopTime := now.Sub(lastLoopStart)
		lastLoopStart = now

		// We use an angle.PlusMinus180 to make sure we do our modulo arithmetic
		// correctly...
		headingEstimate = imuReport.RobotYaw().Sub(initialHeading)

		// Grab the current control values.
		h.controlLock.Lock()
		controls := h.relativeControls
		h.currentHeading = headingEstimate
		h.controlLock.Unlock()

		// Update our target heading accordingly.
		loopTimeSecs := loopTime.Seconds()

		targetHeading = targetHeading.AddFloat(controls.yawRateDegreesPerS * loopTime.Seconds())

		// Cap the difference between our actual angle and the target so that we can't
		// accumulate the desire to spin around and around.
		leadAngle := targetHeading.Sub(headingEstimate).Float()
		const maxLeadAngle = 20.0
		if leadAngle < -maxLeadAngle {
			leadAngle = -maxLeadAngle
			targetHeading = headingEstimate.SubFloat(maxLeadAngle)
		} else if leadAngle > maxLeadAngle {
			leadAngle = maxLeadAngle
			targetHeading = headingEstimate.AddFloat(maxLeadAngle)
		}

		const (
			kp          = 6.0
			ki          = 0.8
			kd          = 0.10
			maxIntegral = 20
			maxD        = 100
		)

		// Calculate the error/derivative/integral.
		headingErrorDegrees := targetHeading.Sub(headingEstimate).Float()
		dHeadingError := (headingErrorDegrees - lastHeadingError) / loopTimeSecs
		if dHeadingError > maxD {
			dHeadingError = maxD
		} else if dHeadingError < -maxD {
			dHeadingError = -maxD
		}

		if math.Abs(headingErrorDegrees) < 5 {
			iHeadingError += headingErrorDegrees * loopTimeSecs
			if iHeadingError > maxIntegral {
				iHeadingError = maxIntegral
			} else if iHeadingError < -maxIntegral {
				iHeadingError = -maxIntegral
			}
		} else {
			iHeadingError = 0
		}

		// Calculate how fast we want the bot as a whole to rotate.
		desiredBotDegreesPS := kp*headingErrorDegrees + ki*iHeadingError + kd*dHeadingError
		if math.Abs(controls.yawRateDegreesPerS) > 30 {
			desiredBotDegreesPS = controls.yawRateDegreesPerS
		}

		rotationMMPerS := desiredBotDegreesPS * chassis.WheelTurningCircleDiaMM / 360
		if rotationMMPerS > maxRotationMMPerS {
			rotationMMPerS = maxRotationMMPerS
		} else if rotationMMPerS < -maxRotationMMPerS {
			rotationMMPerS = -maxRotationMMPerS
		}

		if time.Since(lastPrint) > 300*time.Millisecond {
			fmt.Printf("HH: %v Heading: %.1f Target: %.1f Error: %.1f Int: %.1f D: %.1f -> %.3f\n",
				loopTime, headingEstimate, targetHeading, headingErrorDegrees, iHeadingError, dHeadingError, rotationMMPerS)
		}
		targetThrottle := controls.throttleMMPerS
		maxThrottleDelta := maxThrottleDeltaPerSec * loopTime.Seconds()
		maxTranslationDelta := maxThrottleDelta
		if targetThrottle > filteredThrottle+maxThrottleDelta {
			filteredThrottle += maxThrottleDelta
			//fmt.Printf("HH capping throttle delta, target: %.2f capped: %.2f\n", targetThrottle, filteredThrottle)
		} else if targetThrottle < filteredThrottle-maxThrottleDelta {
			filteredThrottle -= maxThrottleDelta
			//fmt.Printf("HH capping throttle delta, target: %.2f capped: %.2f\n", targetThrottle, filteredThrottle)
		} else {
			filteredThrottle = targetThrottle
		}
		const mecFac = 1.044
		targetTranslation := controls.translationMMPerS * mecFac
		if targetTranslation > filteredTranslation+maxTranslationDelta {
			filteredTranslation += maxTranslationDelta
		} else if targetTranslation < filteredTranslation-maxTranslationDelta {
			filteredTranslation -= maxTranslationDelta
		} else {
			filteredTranslation = targetTranslation
		}

		// Map the values to speeds for each motor.  Motor rotation direction:
		// positive = anti-clockwise.
		throttleRPS := filteredThrottle / chassis.WheelCircumMM
		translationRPS := filteredTranslation / chassis.WheelCircumMM
		rotationRPS := rotationMMPerS / chassis.WheelCircumMM

		if time.Since(lastPrint) > 300*time.Millisecond {
			fmt.Printf("RPS: th=%.2f tr=%.2f ro=%.2f\n", throttleRPS, translationRPS, rotationRPS)
			lastPrint = time.Now()
		}
		var frontLeftRPS float64 = throttleRPS - rotationRPS - translationRPS
		var backLeftRPS float64 = throttleRPS - rotationRPS + translationRPS
		var frontRightRPS float64 = -throttleRPS - rotationRPS - translationRPS
		var backRightRPS float64 = -throttleRPS - rotationRPS + translationRPS

		m := max(frontLeftRPS, frontRightRPS, backLeftRPS, backRightRPS)
		scale := 1.0
		if m > maxRPS {
			scale = maxRPS / m
		}

		fl := picobldc.RPSToMotorSpeed(frontLeftRPS * scale)
		fr := picobldc.RPSToMotorSpeed(frontRightRPS * scale)
		bl := picobldc.RPSToMotorSpeed(backLeftRPS * scale)
		br := picobldc.RPSToMotorSpeed(backRightRPS * scale)

		if err := h.Motors.SetMotorSpeeds(fl, fr, bl, br); err != nil {
			fmt.Println("Failed to set motor speeds:", err)
		}
		lastHeadingError = headingErrorDegrees
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
