package rcmode

import (
	"context"
	"math"
	"sync"

	"fmt"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcheadingholder"
)

type ServoController interface {
	Start(propLock *sync.Mutex, propeller propeller.Interface)
	Stop()

	OnJoystickEvent(event *joystick.Event)
}

type RCMode struct {
	name         string
	startupSound string

	propLock  sync.Mutex // Guards access to the propeller
	Propeller propeller.Interface

	servoController ServoController

	cancel         context.CancelFunc
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

	headingHolder *headingholder.RCHeadingHolder
}

func New(name, startupSound string, propeller propeller.Interface, servoController ServoController) *RCMode {
	r := &RCMode{
		Propeller:       propeller,
		joystickEvents:  make(chan *joystick.Event),
		servoController: servoController,
		name:            name,
		startupSound:    startupSound,
	}
	r.headingHolder = headingholder.New(&r.propLock, propeller)
	return r
}

func (m *RCMode) Name() string {
	return m.name
}

func (m *RCMode) StartupSound() string {
	return m.startupSound
}

func (m *RCMode) Start(ctx context.Context) {
	m.stopWG.Add(2)
	var loopCtx context.Context
	loopCtx, m.cancel = context.WithCancel(ctx)
	go m.loop(loopCtx)
	go m.headingHolder.Loop(loopCtx, &m.stopWG)
}

func (m *RCMode) Stop() {
	m.cancel()
	m.stopWG.Wait()
}

func (m *RCMode) loop(ctx context.Context) {
	defer m.stopWG.Done()

	m.servoController.Start(&m.propLock, m.Propeller)
	defer m.servoController.Stop()

	var leftStickX, leftStickY, rightStickX, rightStickY int16
	var mix = MixAggressive

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-m.joystickEvents:
			switch event.Type {
			case joystick.EventTypeAxis:
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
			case joystick.EventTypeButton:
				switch event.Number {
				case joystick.ButtonL2:
					if event.Value == 1 {
						fmt.Println("Gentle mode")
						mix = MixGentle
					} else {
						fmt.Println("Aggressive mode")
						mix = MixAggressive
					}
				}
			}

			m.servoController.OnJoystickEvent(event)
			yaw, throttle, translate := mix(leftStickX, leftStickY, rightStickX, rightStickY)
			m.headingHolder.SetControlInputs(yaw, throttle, translate)
		}

	}
}

func (m *RCMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}

func MixGentle(lStickX, lStickY, rStickX, rStickY int16) (yaw, throttle, translate float64) {
	const expo = 1.6
	_ = lStickY

	// Put all the values into the range (-1, 1) and apply expo.
	yaw = applyExpo(float64(lStickX)/32767.0, 2.5) / 4
	throttle = applyExpo(float64(rStickY)/-32767.0, expo) / 4
	translate = applyExpo(float64(rStickX)/32767.0, expo) / 4
	return
}

func MixAggressive(lStickX, lStickY, rStickX, rStickY int16) (yaw, throttle, translate float64) {
	const expo = 1.6
	_ = lStickY

	// Put all the values into the range (-1, 1) and apply expo.
	yaw = applyExpo(float64(lStickX)/32767.0, expo)
	throttle = applyExpo(float64(rStickY)/-32767.0, expo)
	translate = applyExpo(float64(rStickX)/32767.0, expo)

	return
}

func applyExpo(value float64, expo float64) float64 {
	absVal := math.Abs(value)
	absExpo := math.Pow(absVal, expo)
	signedExpo := math.Copysign(absExpo, value)
	return signedExpo
}
