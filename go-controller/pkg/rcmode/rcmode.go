package rcmode

import (
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"sync"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"context"
)

type RCMode struct {
	Propeller *propeller.Propeller
	cancel    context.CancelFunc
	stopWG    sync.WaitGroup
	joystickEvents chan *joystick.Event
}

func New(propeller *propeller.Propeller) *RCMode {
	return &RCMode{
		Propeller:propeller,
		joystickEvents: make(chan *joystick.Event),
	}
}

func (m *RCMode) Name() string {
	return "Test mode"
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
		case <- ctx.Done():
			return
		case event := <-m.joystickEvents:
				if event.Type == joystick.EventTypeAxis && event.Number == joystick.AxisLStickX {
					stickX = event.Value
				} else if event.Type == joystick.EventTypeAxis && event.Number == joystick.AxisLStickY {
					stickY = event.Value
				} else {
					continue
				}
			fl, fr, bl, br := LinearMixer(stickX, stickY)
			err := m.Propeller.SetMotorSpeeds(fl,fr,bl,br)
			if err != nil {
				panic(err)
			}
		}

	}
}

func (m *RCMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}

func LinearMixer(stickX, stickY int16) (fl, fr, bl, br int8) {
	stickY = stickY >> 9
	fl = int8(-stickY)
	fr = int8(-stickY)
	bl = int8(-stickY)
	br = int8(-stickY)
	return
}
