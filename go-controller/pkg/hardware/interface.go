package hardware

import (
	"context"
	"fmt"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder/angle"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/picobldc"
	"sync"
	"time"
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
	SetThrottle(float64)
	Wait(ctx context.Context) (residual float64, err error)
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
