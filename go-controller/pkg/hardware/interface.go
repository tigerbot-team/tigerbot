package hardware

import (
	"context"
	"fmt"
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
	CurrentHeading() float64
	CurrentDistanceReadings(revision revision) DistanceReadings
	CurrentMotorDistances() (l, r float64)

	SetServo(port int, value float64)
	SetPWM(port int, value float64)

	PlaySound(path string)

	Shutdown()
}

type RawControl interface {
	SetMotorSpeeds(left, right int8)
}

type HeadingAbsolute interface {
	SetHeading(desiredHeaading float64)
	AddHeadingDelta(delta float64)
	SetThrottle(float64)
	Wait(ctx context.Context) (residual float64, err error)
}

type HeadingRelative interface {
	SetYawAndThrottle(yaw, throttle float64)
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
	SetMotorSpeeds(left, right int8)
	SetServo(n int, value float64)
	SetPWM(n int, value float64)
	CurrentDistanceReadings(revision revision) DistanceReadings
	CurrentMotorDistances() (l, r float64)
	Loop(context context.Context, initDone *sync.WaitGroup)
}
