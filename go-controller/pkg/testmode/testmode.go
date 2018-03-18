package testmode

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"gocv.io/x/gocv"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
	mux2 "github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
)

type TestMode struct {
	Propeller propeller.Interface
	context context.Context
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
	t.context = loopCtx
	go t.loop(loopCtx)
}

func (t *TestMode) Stop() {
	t.cancel()
	t.stopWG.Wait()
}

func (t *TestMode) loop(ctx context.Context) {
	defer t.stopWG.Done()
	<-ctx.Done()
}

func (t *TestMode) testMotors(ctx context.Context) {
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
			go t.takePicture()
		case joystick.ButtonR2:
			fmt.Println("TestMode: button R2 pushed, benchmarking camera")
			go t.benchmarkPicture()
		case joystick.ButtonSquare:
			fmt.Println("TestMode: square pushed, testing sensors")
			go t.testSensors(t.context)
		case joystick.ButtonTriangle:
			fmt.Println("TestMode: triangle pushed, testing sensors")
			t.stopWG.Add(1)
			go t.testMotors(t.context)
		}
	}
}

func (t *TestMode) testSensors(ctx context.Context) {
	m, err := mux2.New("/dev/i2c-1")
	if err != nil {
		fmt.Println("Failed to open mux", err)
		return
	}

	var tofs []tofsensor.Interface
	defer func() {
		for _, tof := range tofs {
			tof.Close()
		}
	}()
	for _, port := range []int{mux2.BusTOF1, mux2.BusTOF2, mux2.BusTOF3} {
		tof, err := tofsensor.NewMuxed("/dev/i2c-1", 0x29, m, port)
		if err != nil {
			fmt.Println("Failed to open sensor", err)
			return
		}
		err = tof.StartContinuousMeasurements()
		if err != nil {
			fmt.Println("Failed to start continuous measurements", err)
			return
		}
		tofs = append(tofs, tof)
	}
	for ctx.Err() == nil {
		for j, tof := range tofs {
			reading := "-"
			readingInMM, err := tof.GetNextContinuousMeasurement()
			if err == tofsensor.ErrMeasurementInvalid {
				reading = "<invalid>"
			} else if err != nil {
				reading = "<failed>"
			} else {
				reading = fmt.Sprintf("%dmm", readingInMM)
			}
			fmt.Printf("%d: %10s ", j, reading)
			if ctx.Err() != nil {
				return
			}
		}
		fmt.Println()
		time.Sleep(10 * time.Millisecond)
	}
}

func (t *TestMode) takePicture() {
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

func (t *TestMode) benchmarkPicture() {
	deviceID := 0
	webcam, err := gocv.VideoCaptureDevice(int(deviceID))
	if err != nil {
		fmt.Printf("error opening video capture device: %v\n", deviceID)
		return
	}
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()

	startTime := time.Now()

	for i := 0; i < 100; i++ {
		if ok := webcam.Read(img); !ok {
			fmt.Printf("cannot read device %d\n", deviceID)
			return
		}
		if img.Empty() {
			fmt.Printf("no image on device %d\n", deviceID)
			return
		}
	}

	fmt.Printf("TestMode: time per image %v\n", time.Since(startTime) / 100)
}
