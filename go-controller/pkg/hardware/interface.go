package hardware

import "time"

type Interface interface {
	// Enter a particular motor control mode (previous mode will be stopped if needed).
	StartRawControlMode() RawControl
	StartHeadingHoldMode() HeadingAbsolute
	StartYawAndThrottleMode() HeadingRelative
	StopMotorControl()

	// Read the current state of the hardware.  Reads the current best guess from cache.
	CurrentHeading() float64
	CurrentDistanceReadings() DistanceReadings

	SetServo(n int, value uint8)

	PlaySound(path string)
}

type RawControl interface {
	SetMotorSpeeds(left, right int8)
}

type HeadingAbsolute interface {
	SetHeading(desiredHeaading float64)
	AddHeadingDelta(delta float64)
	SetThrottle(float64)
}

type HeadingRelative interface {
	SetYawAndThrottle(yaw, throttle float64)
}

type DistanceReadings struct {
	CaptureTime time.Time

	// Clockwise from left-side-rear to right-side-rear
	Readings []Reading
}

type Reading struct {
	DistanceMM int
	Error      error
}

type I2CInterface interface {
	SetMotorSpeeds(left, right int8)
	SetServo(n int, value uint8)
	CurrentDistanceReadings() DistanceReadings
}
