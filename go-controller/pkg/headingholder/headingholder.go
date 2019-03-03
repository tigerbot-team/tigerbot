package headingholder

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/imu"
)

func New(prop RawControl) *HeadingHolder {
	return &HeadingHolder{
		Motors: prop,
	}
}

type RawControl interface {
	SetMotorSpeeds(left, right int8)
}

type HeadingHolder struct {
	Motors RawControl

	controlLock    sync.Mutex
	yaw, throttle  float64
	currentHeading float64
}

func (y *HeadingHolder) SetYawAndThrottle(yaw, throttle float64) {
	y.controlLock.Lock()
	defer y.controlLock.Unlock()

	y.yaw = yaw
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

	const imuDT = 1 * time.Millisecond
	const targetLoopDT = 20 * time.Millisecond

	ticker := time.NewTicker(targetLoopDT)
	defer ticker.Stop()

	var headingEstimate float64
	var targetHeading float64
	var filteredTranslation, filteredThrottle float64
	var motorRotationSpeed float64
	var lastHeadingError float64
	var iHeadingError float64

	const (
		kp                     = 0.010
		ki                     = 0.0
		kd                     = -0.00007
		maxIntegral            = .1
		maxMotorSpeed          = 2.0
		maxThrottleDeltaPerSec = 1
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
		targetYaw := y.yaw
		targetThrottle := y.throttle
		y.currentHeading = headingEstimate
		y.controlLock.Unlock()

		// Update our target heading accordingly.
		loopTimeSecs := loopTime.Seconds()

		// Avoid letting the yaw lead the heading too much.
		targetHeading += loopTimeSecs * targetYaw * 300

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
		if motorRotationSpeed > maxMotorSpeed {
			motorRotationSpeed = maxMotorSpeed
		} else if motorRotationSpeed < -maxMotorSpeed {
			motorRotationSpeed = -maxMotorSpeed
		}

		fmt.Printf("HH: %v Heading: %.1f Target: %.1f Error: %.1f Int: %.1f D: %.1f -> %.1f\n",
			loopTime, headingEstimate, targetHeading, headingError, iHeadingError, dHeadingError, motorRotationSpeed)

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
		frontLeft := filteredThrottle + motorRotationSpeed + filteredTranslation
		frontRight := filteredThrottle - motorRotationSpeed - filteredTranslation
		backLeft := filteredThrottle + motorRotationSpeed - filteredTranslation
		backRight := filteredThrottle - motorRotationSpeed + filteredTranslation

		m1 := math.Max(frontLeft, frontRight)
		m2 := math.Max(backLeft, backRight)
		m := math.Max(m1, m2)
		scale := 1.0
		if m > 1 {
			scale = 1.0 / m
		}

		fl := scaleAndClamp(frontLeft*scale, 127)
		fr := scaleAndClamp(frontRight*scale, 127)
		//bl := scaleAndClamp(backLeft*scale, 127)
		//br := scaleAndClamp(backRight*scale, 127)

		y.Motors.SetMotorSpeeds(fl, fr)

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
