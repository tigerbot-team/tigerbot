package rcmode

import (
	"context"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/screen"
	"log"
	"math"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcmode/servo"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"

	"fmt"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
)

type RCMode struct {
	name         string
	startupSound string

	propLock sync.Mutex // Guards access to the picobldc
	hardware hardware.Interface

	servoController servo.ServoController

	cancel         context.CancelFunc
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event
}

func New(
	name,
	startupSound string,
	hw hardware.Interface,
	servoController servo.ServoController,
) *RCMode {
	r := &RCMode{
		hardware:        hw,
		joystickEvents:  make(chan *joystick.Event),
		servoController: servoController,
		name:            name,
		startupSound:    startupSound,
	}
	return r
}

func (m *RCMode) Name() string {
	return m.name
}

func (m *RCMode) StartupSound() string {
	return m.startupSound
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
	fmt.Println("RCMode main loop started")

	screen.SetEnabled(true)

	fmt.Println("RCMode Starting servo controller")
	m.servoController.Start(m.hardware)
	defer m.servoController.Stop()
	fmt.Println("RCMode Started servo controller")

	var leftStickX, leftStickY, rightStickX, rightStickY int16
	var mix = MixAggressive

	fmt.Println("RCMode taking control of motors")
	motorController := m.hardware.StartYawAndThrottleMode()
	defer m.hardware.StopMotorControl()

	screenEnabled := true
	screenTicker := time.NewTicker(200 * time.Millisecond)
	defer screenTicker.Stop()

	showAimC := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-showAimC:

				_, err := challengemode.CameraExecute(log.Printf, "show-aim-point")
				if err != nil {
					fmt.Printf("Failed to show image: %v\n", err)
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("RCMode context done")
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
				case joystick.ButtonTriangle:
					if event.Value == 1 {
						screenEnabled = !screenEnabled
						screen.SetEnabled(screenEnabled)
					}
				}
			}

			m.servoController.OnJoystickEvent(event)
			yaw, throttle, translation := mix(leftStickX, leftStickY, rightStickX, rightStickY)
			motorController.SetYawAndThrottle(-yaw, throttle, translation)

			//m.hardware.SetServo(8, clamp(0.3+throttle/2-yaw/3, 0.2, 1))  // 0 is arm down, 1 is arm up
			//m.hardware.SetServo(10, clamp(0.7-throttle/2-yaw/3, 0, 0.8)) // 0 is arm up, 1 is arm down
			//m.hardware.SetServo(9, clamp(-yaw/2+0.5, 0.25, 0.75))        // 0.25 is right, 0.75 is left
		case <-screenTicker.C:
			if screenEnabled {
				continue
			}
			select {
			case showAimC <- struct{}{}:
			default:
			}
		}
	}
}

func (m *RCMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}

func MixGentle(lStickX, lStickY, rStickX, rStickY int16) (yaw, throttle, translation float64) {
	const expo = 1.6
	_ = lStickY
	_ = rStickX

	// Put all the values into the range (-1, 1) and apply expo.
	yaw = applyExpo(float64(lStickX)/32767.0, expo) / 3
	throttleStick := rStickY
	if math.Abs(float64(lStickY)) > math.Abs(float64(rStickY)) {
		throttleStick = lStickY
	}
	throttle = applyExpo(float64(throttleStick)/-32767.0, expo) / 3
	translation = applyExpo(float64(rStickX)/-32767.0, expo) / 2
	return
}

func MixAggressive(lStickX, lStickY, rStickX, rStickY int16) (yaw, throttle, translation float64) {
	const expo = 1.6
	_ = lStickY
	_ = rStickX

	// Put all the values into the range (-1, 1) and apply expo.
	yaw = applyExpo(float64(lStickX)/32767.0, expo)
	throttleStick := rStickY
	if math.Abs(float64(lStickY)) > math.Abs(float64(rStickY)) {
		throttleStick = lStickY
	}
	throttle = applyExpo(float64(throttleStick)/-32767.0, expo)
	translation = applyExpo(float64(rStickX)/-32767.0, expo)

	return
}

func applyExpo(value float64, expo float64) float64 {
	absVal := math.Abs(value)
	absExpo := math.Pow(absVal, expo)
	signedExpo := math.Copysign(absExpo, value)
	return signedExpo
}

func clamp(f, min, max float64) float64 {
	if f < min {
		return min
	}
	if f > max {
		return max
	}
	return f
}
