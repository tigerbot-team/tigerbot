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

	var stickX, stickY int16


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
			if event.Type == joystick.EventTypeAxis && event.Number == joystick.AxisLStickX {
				stickX = event.Value
			} else if event.Type == joystick.EventTypeAxis && event.Number == joystick.AxisLStickY {
				stickY = event.Value
			} else {
				continue
			}
			fl, fr, bl, br := Mix(stickX, stickY)
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

func Mix(stickX, stickY int16) (fl, fr, bl, br int8) {
	yaw := float64(stickX) / 32767.0
	throttle := float64(stickY) / -32767.0

	yawAbs := math.Abs(yaw)
	throttleAbs := math.Abs(throttle)

	const expo = 2.0
	yawAbsExpo := math.Pow(yawAbs, expo)
	throttleAbsExpo := math.Pow(throttleAbs, expo)

	yawExpo := math.Copysign(yawAbsExpo, yaw)
	throttleExpo := math.Copysign(throttleAbsExpo, throttle)

	left := throttleExpo + yawExpo
	right := throttleExpo - yawExpo

	scaledLF := int8(left * 127 / 2)
	scaledRF := int8(right * 127 / 2)
	scaledLB := int8(left * 80 / 2)
	scaledRB := int8(right * 80 / 2)

	stickY = stickY >> 9
	fl = scaledLF
	fr = scaledRF
	bl = scaledLB
	br = scaledRB
	return
}
