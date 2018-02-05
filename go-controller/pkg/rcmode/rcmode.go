package rcmode

import (
	"context"
	"math"
	"sync"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
)

type RCMode struct {
	Propeller      *propeller.Propeller
	cancel         context.CancelFunc
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event
}

func New(propeller *propeller.Propeller) *RCMode {
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

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-m.joystickEvents:
			if event.Type == joystick.EventTypeAxis && event.Number == joystick.AxisLStickX {
				stickX = event.Value
			} else if event.Type == joystick.EventTypeAxis && event.Number == joystick.AxisLStickY {
				stickY = event.Value
			} else {
				continue
			}
			fl, fr, bl, br := Mix(stickX, stickY)
			err := m.Propeller.SetMotorSpeeds(fl, fr, bl, br)
			if err != nil {
				panic(err)
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

	scaledL := int8(left * 127 / 2)
	scaledR := int8(right * 127 / 2)

	stickY = stickY >> 9
	fl = scaledL
	fr = scaledR
	bl = scaledL
	br = scaledR
	return
}
