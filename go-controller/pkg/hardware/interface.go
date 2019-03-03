package hardware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
)

type Interface interface {
	Start(ctx context.Context)

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

func (r Reading) String() string {
	if r.Error != nil {
		return "<failed>"
	}
	if r.DistanceMM == tofsensor.RangeTooFar {
		return ">2000mm"
	}
	return fmt.Sprintf("%dmm", r.DistanceMM)
}

type I2CInterface interface {
	SetMotorSpeeds(left, right int8)
	SetServo(n int, value uint8)
	CurrentDistanceReadings() DistanceReadings
	Loop(context context.Context, initDone *sync.WaitGroup)
}
