package headingholder

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/bno08x"
)

func New(motors RawControl) *HeadingHolder {
	return &HeadingHolder{
		Motors: motors,
	}
}

type RawControl interface {
	SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error
}

type HeadingHolder struct {
	Motors RawControl

	controlLock       sync.Mutex
	yawRate, throttle float64
	currentHeading    float64
}

func (y *HeadingHolder) SetYawAndThrottle(yaw, throttle float64) {
	y.controlLock.Lock()
	defer y.controlLock.Unlock()

	y.yawRate = yaw
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

	m := bno08x.New()
	go m.LoopReadingReports(cxt)

	lastPrint := time.Now()
	var lastIMUReport bno08x.IMUReport
	for {
		if cxt.Err() != nil {
			fmt.Println("Context finished.")
			return
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

	var headingEstimate float64
	var targetHeading float64 = lastIMUReport.YawDegrees()
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
	var lastLoopStart = time.Now()
	for cxt.Err() == nil {
		// This should pop every 10ms
		imuReport := m.WaitForReportAfter(lastIMUReport.Time)

		now := time.Now()
		loopTime := now.Sub(lastLoopStart)
		lastLoopStart = now

		// The IMU gives us absolute heading in range -180 to +180.  Shift that
		// to 0-360
		headingEstimate = imuReport.YawDegrees()

		// Grab the current control values.
		y.controlLock.Lock()
		targetYawRate := y.yawRate
		targetThrottle := y.throttle
		y.currentHeading = headingEstimate
		y.controlLock.Unlock()

		// Update our target heading accordingly.
		loopTimeSecs := loopTime.Seconds()

		// Avoid letting the yaw lead the heading too much.
		const yawRateFactor = 300
		targetHeading = clampDegrees(targetHeading + loopTimeSecs*targetYawRate*yawRateFactor)
		leadDegrees := clampDegrees(targetHeading - headingEstimate)

		const maxLeadDegrees = 20
		if leadDegrees > maxLeadDegrees {
			targetHeading = clampDegrees(headingEstimate + maxLeadDegrees)
		} else if leadDegrees < -maxLeadDegrees {
			targetHeading = clampDegrees(headingEstimate - maxLeadDegrees)
		}

		// Calculate the error/derivative/integral.
		headingError := clampDegrees(targetHeading - headingEstimate)
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

		// Map the values to speeds for each motor.  Motor rotation direction:
		// positive = anti-clockwise.
		frontLeft := filteredThrottle + motorRotationSpeed + filteredTranslation
		frontRight := -filteredThrottle + motorRotationSpeed - filteredTranslation
		backLeft := filteredThrottle + motorRotationSpeed - filteredTranslation
		backRight := -filteredThrottle + motorRotationSpeed + filteredTranslation

		m := max(frontLeft, frontRight, backLeft, backRight)
		scale := 1.0
		if m > 1 {
			scale = 1.0 / m
		}

		const multiplier = 0xffff
		fl := scaleAndClamp(frontLeft*scale, multiplier)
		fr := scaleAndClamp(frontRight*scale, multiplier)
		bl := scaleAndClamp(backLeft*scale, multiplier)
		br := scaleAndClamp(backRight*scale, multiplier)

		if err := y.Motors.SetMotorSpeeds(fl, fr, bl, br); err != nil {
			fmt.Println("Failed to set motor speeds:", err)
		}

		lastHeadingError = headingError
		lastIMUReport = imuReport
	}
	if err := y.Motors.SetMotorSpeeds(0, 0, 0, 0); err != nil {
		fmt.Println("Failed to set motor speeds:", err)
	}
}

// clampDegrees shifts d into the range (-180, 180]
func clampDegrees(d float64) float64 {
	d = math.Mod(d, 360)
	if d <= -180 {
		d += 360
	} else if d > 180 {
		d -= 360
	}
	return d
}

func scaleAndClamp(value, multiplier float64) int16 {
	multiplied := value * multiplier
	if multiplied <= math.MinInt16 {
		return math.MinInt16
	}
	if multiplied >= math.MaxInt16 {
		return math.MaxInt16
	}
	return int16(multiplied)
}
