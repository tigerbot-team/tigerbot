package hardware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder/angle"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/picobldc"
)

type Interface interface {
	Start(ctx context.Context)

	// Enter a particular motor control mode (previous mode will be stopped if needed).
	StartRawControlMode() RawControl
	StartHeadingHoldMode() HeadingAbsolute
	StartYawAndThrottleMode() HeadingRelative
	StopMotorControl()

	// Read the current state of the hardware.  Reads the current best guess from cache.
	CurrentHeading() angle.PlusMinus180
	CurrentDistanceReadings(revision revision) DistanceReadings
	AccumulatedRotations() picobldc.PerMotorVal[float64]

	SetServo(port int, value float64)
	SetPWM(port int, value float64)

	PlaySound(path string)
}

type RawControl interface {
	SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error
}

type HeadingAbsolute interface {
	SetHeading(desiredHeaading float64)
	AddHeadingDelta(delta float64)
	SetThrottle(throttleMMPerS float64)
	Wait(ctx context.Context) (residual float64, err error)

	// SetThrottleWithAngle is like SetThrottle with an angled
	//displacement - i.e. not changing the direction that the bot
	//is facing (aka the "heading"), but using mecanum wheels to
	// move other than just straight ahead.
	//
	// First arg `throttle` is the same as in `SetThrottle`,
	// i.e. a measurement of speed.
	//
	// Second arg `angle` is in degrees, measured CCW
	// w.r.t. straight ahead.
	//
	// (So `SetThrottleWithAngle(throttle, 0)` should be
	// equivalent to `SetThrottle(throttle)`.
	SetThrottleWithAngle(throttleMMPerS float64, angle float64)
}

type HeadingRelative interface {
	SetYawAndThrottle(yawRate, throttle, translation float64)
}

type revision uint64

const (
	RevCurrent = 0
)

type DistanceReadings struct {
	CaptureTime time.Time

	// Clockwise from left-side-rear to right-side-rear
	Readings []Reading
	Revision revision
}

var startTime = time.Now()

func (d DistanceReadings) String() string {
	return fmt.Sprintf("T+%.4f %v", d.CaptureTime.Sub(startTime).Seconds(), d.Readings)
}

type Reading struct {
	DistanceMM int
	Error      error `json:"-"`
}

func (r Reading) String() string {
	//if r.Error != nil {
	//	return "<failed>"
	//}
	//if r.DistanceMM == tofsensor.RangeTooFar {
	//	return ">2000mm"
	//}
	return fmt.Sprintf("%dmm", r.DistanceMM)
}

type I2CInterface interface {
	SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error
	SetServo(n int, value float64)
	SetPWM(n int, value float64)
	CurrentDistanceReadings(revision revision) DistanceReadings
	AccumulatedRotations() picobldc.PerMotorVal[float64]
	Loop(context context.Context, initDone *sync.WaitGroup)
}
