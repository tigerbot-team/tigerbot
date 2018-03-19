package rcmode

import (
	"context"
	"math"
	"sync"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"time"
	"fmt"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
)

type RCMode struct {
	Propeller      propeller.Interface
	cancel         context.CancelFunc
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event
}

func New(propeller propeller.Interface) *RCMode {
	return &RCMode{
		Propeller:      propeller,
		joystickEvents: make(chan *joystick.Event),
	}
}

func (m *RCMode) Name() string {
	return "RC mode"
}

func (m *RCMode) Start(ctx context.Context) {
	m.stopWG.Add(1)
	var loopCtx context.Context
	loopCtx, m.cancel = context.WithCancel(ctx)
	go m.loop(loopCtx)
}

func (m *RCMode) Stop() {
	m.cancel()
	m.stopWG.Wait()
}

func (m *RCMode) loop(ctx context.Context) {
	defer m.stopWG.Done()

	var leftStickX, leftStickY, rightStickX, rightStickY int16


	mx, err := mux.New("/dev/i2c-1")
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
	for _, port := range []int{mux.BusTOF1, mux.BusTOF2, mux.BusTOF3} {
		tof, err := tofsensor.NewMuxed("/dev/i2c-1", 0x29, mx, port)
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

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
		case event := <-m.joystickEvents:
			if event.Type == joystick.EventTypeAxis {
				switch event.Number {
				case joystick.AxisLStickX:
					leftStickX = event.Value
				case joystick.AxisLStickY:
					leftStickY = event.Value
				case joystick.AxisRStickX:
					rightStickX = event.Value
				case joystick.AxisRStickY:
					rightStickY = event.Value
				}
			}
			fl, fr, bl, br := Mix(leftStickX, leftStickY, rightStickX, rightStickY)
			for {
				err := m.Propeller.SetMotorSpeeds(fl, fr, bl, br)
				if err == nil {
					break
				}
				time.Sleep(1 * time.Millisecond)
			}
		}

	}
}

func (m *RCMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}

func Mix(lStickX, lStickY, rStickX, rStickY int16) (fl, fr, bl, br int8) {
	const expo = 2.0
	_ = lStickY

	// Put all the values into the range (-1, 1) and apply expo.
	yawExpo := applyExpo(float64(lStickX) / 32767.0, expo)
	throttleExpo := applyExpo(float64(rStickY) / -32767.0, expo)
	translateExpo := applyExpo(float64(rStickX) / 32767.0, expo)

	// Map the values to speeds for each motor.
	frontLeft := throttleExpo + yawExpo + translateExpo
	frontRight := throttleExpo - yawExpo - translateExpo
	backLeft := throttleExpo + yawExpo - translateExpo
	backRight := throttleExpo - yawExpo + translateExpo

	fl = scaleAndClamp(frontLeft, 127)
	fr = scaleAndClamp(frontRight, 127)
	bl = scaleAndClamp(backLeft, 127)
	br = scaleAndClamp(backRight, 127)
	return
}

func applyExpo(value float64, expo float64) float64 {
	absVal := math.Abs(value)
	absExpo := math.Pow(absVal, expo)
	signedExpo := math.Copysign(absExpo, value)
	return signedExpo
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
