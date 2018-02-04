package testmode

import (
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"sync"
	"fmt"
	"time"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"context"
)

type TestMode struct {
	Propeller *propeller.Propeller
	cancel    context.CancelFunc
	stopWG    sync.WaitGroup
}

func (t *TestMode) Name() string {
	return "Test mode"
}

func (t *TestMode) Start(ctx context.Context) {
	t.stopWG.Add(1)
	var loopCtx context.Context
	loopCtx, t.cancel = context.WithCancel(ctx)
	go t.loop(loopCtx)
}

func (t *TestMode) Stop() {
	t.cancel()
	t.stopWG.Wait()
}

func (t *TestMode) loop(ctx context.Context) {
	defer t.stopWG.Done()
	for {
		fmt.Println("TestMode: Front left")
		err := t.Propeller.SetMotorSpeeds(2, 0, 0, 0)
		if err != nil {
			panic(err)
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(1*time.Second)
		fmt.Println("TestMode: Front right")
		err = t.Propeller.SetMotorSpeeds(0, 2, 0, 0)
		if err != nil {
			panic(err)
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(1*time.Second)
		fmt.Println("TestMode: Back left")
		err = t.Propeller.SetMotorSpeeds(0, 0, 2, 0)
		if err != nil {
			panic(err)
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(1*time.Second)
		fmt.Println("TestMode: Back right")
		err = t.Propeller.SetMotorSpeeds(0, 0, 0, 2)
		if err != nil {
			panic(err)
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(1*time.Second)


		fmt.Println("TestMode: Turning off motors")
		err = t.Propeller.SetMotorSpeeds(0,0, 0, 0)
		if err != nil {
			panic(err)
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(5*time.Second)
	}
}

func (t *TestMode) OnJoystickEvent(event *joystick.Event) {
	fmt.Println("TestMode: Joystick event", event)
}

type PauseMode struct {
	Propeller *propeller.Propeller
}

func (t *PauseMode) Name() string{
	return "Pause mode"
}

func (t *PauseMode) Start(ctx context.Context) {
	t.Propeller.SetMotorSpeeds(0,0,0,0)
}

func (t *PauseMode) Stop() {
}
