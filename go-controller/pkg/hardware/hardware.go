package hardware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/screen"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/sound"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholderabs"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder"
)

type Hardware struct {
	i2c I2CInterface

	soundsToPlay chan string

	cancelCurrentControlMode context.CancelFunc
	currentControlModeDone   sync.WaitGroup
	imu                      interface {
		CurrentHeading() float64
	}
}

func New() *Hardware {
	i2c := NewI2CController()
	return &Hardware{
		i2c:          i2c,
		soundsToPlay: sound.InitSound(),
	}
}

var _ Interface = (*Hardware)(nil)

func (h *Hardware) Start(ctx context.Context) {
	var initDone sync.WaitGroup
	go screen.LoopUpdatingScreen(ctx)
	initDone.Add(1)
	go h.i2c.Loop(ctx, &initDone)
	initDone.Wait()
}

func (h *Hardware) StartRawControlMode() RawControl {
	// Raw mode doesn't have any state so pass through.
	h.StopMotorControl()
	return h.i2c
}

func (h *Hardware) StartHeadingHoldMode() HeadingAbsolute {
	h.StopMotorControl()

	var ctx context.Context
	ctx, h.cancelCurrentControlMode = context.WithCancel(context.Background())

	hh := headingholderabs.New(h.i2c)
	h.currentControlModeDone.Add(1)
	go hh.Loop(ctx, &h.currentControlModeDone)
	h.imu = hh
	return hh
}

func (h *Hardware) StartYawAndThrottleMode() HeadingRelative {
	h.StopMotorControl()

	var ctx context.Context
	ctx, h.cancelCurrentControlMode = context.WithCancel(context.Background())

	hh := headingholder.New(h.i2c)
	h.currentControlModeDone.Add(1)
	go hh.Loop(ctx, &h.currentControlModeDone)
	h.imu = hh
	return hh
}

func (h *Hardware) StopMotorControl() {
	if h.cancelCurrentControlMode != nil {
		fmt.Println("HW: Stopping motor control")
		h.cancelCurrentControlMode()
		h.currentControlModeDone.Wait()
		h.cancelCurrentControlMode = nil
		fmt.Println("HW: Stopped motor control")
	}
	h.imu = nil
	h.i2c.SetMotorSpeeds(0, 0)
	time.Sleep(30 * time.Millisecond)
}

func (h *Hardware) CurrentHeading() float64 {
	if h.imu == nil {
		return 0
	}
	// FIXME Run IMU all the time, not just when heading holding.
	return h.imu.CurrentHeading()
}

func (h *Hardware) CurrentDistanceReadings(rev revision) DistanceReadings {
	return h.i2c.CurrentDistanceReadings(rev)
}

func (h *Hardware) CurrentMotorDistances() (l, r float64) {
	return h.i2c.CurrentMotorDistances()
}

func (h *Hardware) SetServo(n int, value float64) {
	h.i2c.SetServo(n, value)
}

func (h *Hardware) SetPWM(n int, value float64) {
	h.i2c.SetPWM(n, value)
}

func (h *Hardware) PlaySound(path string) {
	defer func() {
		recover() // Don't die if the channel is already closed.
	}()
	select {
	case h.soundsToPlay <- path:
		return
	case <-time.After(10 * time.Millisecond):
		fmt.Println("Timed out trying to play sound: ", path)
	}
}

func (h *Hardware) Shutdown() {
	h.StopMotorControl()
	close(h.soundsToPlay)
}
