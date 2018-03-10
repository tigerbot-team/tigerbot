package testmode

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"gocv.io/x/gocv"
)

type TestMode struct {
	Propeller propeller.Interface
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
		time.Sleep(1 * time.Second)
		fmt.Println("TestMode: Front right")
		err = t.Propeller.SetMotorSpeeds(0, 2, 0, 0)
		if err != nil {
			panic(err)
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(1 * time.Second)
		fmt.Println("TestMode: Back left")
		err = t.Propeller.SetMotorSpeeds(0, 0, 2, 0)
		if err != nil {
			panic(err)
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(1 * time.Second)
		fmt.Println("TestMode: Back right")
		err = t.Propeller.SetMotorSpeeds(0, 0, 0, 2)
		if err != nil {
			panic(err)
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(1 * time.Second)

		fmt.Println("TestMode: Turning off motors")
		err = t.Propeller.SetMotorSpeeds(0, 0, 0, 0)
		if err != nil {
			panic(err)
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(5 * time.Second)
	}
}

func (t *TestMode) OnJoystickEvent(event *joystick.Event) {
	fmt.Println("TestMode: Joystick event", event)

	if event.Type == joystick.EventTypeButton && event.Value == 1 {
		switch event.Number {
		case joystick.ButtonR1:
			fmt.Println("TestMode: button R1 pushed, taking a picture")

			deviceID := 0
			saveFile := "/tmp/image.jpg"

			webcam, err := gocv.VideoCaptureDevice(int(deviceID))
			if err != nil {
				fmt.Printf("error opening video capture device: %v\n", deviceID)
				return
			}
			defer webcam.Close()

			img := gocv.NewMat()
			defer img.Close()

			if ok := webcam.Read(img); !ok {
				fmt.Printf("cannot read device %d\n", deviceID)
				return
			}
			if img.Empty() {
				fmt.Printf("no image on device %d\n", deviceID)
				return
			}

			success := gocv.IMWrite(saveFile, img)
			fmt.Printf("TestMode: wrote image? %v\n", success)
		}
	}
}
