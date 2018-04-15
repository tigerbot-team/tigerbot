package rcmode

import (
	"context"
	"math"
	"sync"

	"fmt"

	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/imu"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
)

type ServoController interface {
	Start(propLock *sync.Mutex, propeller propeller.Interface)
	Stop()

	OnJoystickEvent(event *joystick.Event)
}

type RCMode struct {
	name string

	propLock  sync.Mutex // Guards access to the propeller
	Propeller propeller.Interface

	servoController ServoController

	cancel         context.CancelFunc
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

	yawMaintainer *YawMaintainer
}

func New(name string, propeller propeller.Interface, servoController ServoController) *RCMode {
	r := &RCMode{
		Propeller:       propeller,
		joystickEvents:  make(chan *joystick.Event),
		servoController: servoController,
		name:            name,
	}
	r.yawMaintainer = &YawMaintainer{
		Propeller: propeller,
		i2cLock:   &r.propLock,
	}
	return r
}

func (m *RCMode) Name() string {
	return m.name
}

func (m *RCMode) Start(ctx context.Context) {
	m.stopWG.Add(2)
	var loopCtx context.Context
	loopCtx, m.cancel = context.WithCancel(ctx)
	go m.loop(loopCtx)
	go m.yawMaintainer.loop(loopCtx, &m.stopWG)
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

			fl, fr, bl, br := mix(leftStickX, leftStickY, rightStickX, rightStickY)
			m.propLock.Lock()
			err := m.Propeller.SetMotorSpeeds(fl, fr, bl, br)
			m.propLock.Unlock()
			if err != nil {
				fmt.Println("Failed to set motor speeds!", err)
			}
		}

	}
}

func (m *RCMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}

func MixGentle(lStickX, lStickY, rStickX, rStickY int16) (fl, fr, bl, br int8) {
	const expo = 1.6
	_ = lStickY

	// Put all the values into the range (-1, 1) and apply expo.
	yawExpo := applyExpo(float64(lStickX)/32767.0, 2.5)
	throttleExpo := applyExpo(float64(rStickY)/-32767.0, expo)
	translateExpo := applyExpo(float64(rStickX)/32767.0, expo)

	// Map the values to speeds for each motor.
	frontLeft := throttleExpo + yawExpo + translateExpo
	frontRight := throttleExpo - yawExpo - translateExpo
	backLeft := throttleExpo + yawExpo - translateExpo
	backRight := throttleExpo - yawExpo + translateExpo

	m1 := math.Max(frontLeft, frontRight)
	m2 := math.Max(backLeft, backRight)
	m := math.Max(m1, m2)
	scale := 1.0
	if m > 1 {
		scale = 1.0 / m
	}

	fl = scaleAndClamp(frontLeft*scale, 32)
	fr = scaleAndClamp(frontRight*scale, 32)
	bl = scaleAndClamp(backLeft*scale, 32)
	br = scaleAndClamp(backRight*scale, 32)
	return
}

func MixAggressive(lStickX, lStickY, rStickX, rStickY int16) (fl, fr, bl, br int8) {
	const expo = 1.6
	_ = lStickY

	// Put all the values into the range (-1, 1) and apply expo.
	yawExpo := applyExpo(float64(lStickX)/32767.0, 2.5)
	throttleExpo := applyExpo(float64(rStickY)/-32767.0, expo)
	translateExpo := applyExpo(float64(rStickX)/32767.0, expo)

	// Map the values to speeds for each motor.
	frontLeft := throttleExpo + yawExpo + translateExpo
	frontRight := throttleExpo - yawExpo - translateExpo
	backLeft := throttleExpo + yawExpo - translateExpo
	backRight := throttleExpo - yawExpo + translateExpo

	m1 := math.Max(frontLeft, frontRight)
	m2 := math.Max(backLeft, backRight)
	m := math.Max(m1, m2)
	scale := 1.0
	if m > 1 {
		scale = 1.0 / m
	}

	fl = scaleAndClamp(frontLeft*scale, 127)
	fr = scaleAndClamp(frontRight*scale, 127)
	bl = scaleAndClamp(backLeft*scale, 127)
	br = scaleAndClamp(backRight*scale, 127)
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

type YawMaintainer struct {
	i2cLock   *sync.Mutex // Guards access to the propeller
	Propeller propeller.Interface

	controlLock                sync.Mutex
	yaw, throttle, translation float64
}

func (y *YawMaintainer) SetControlInputs(yaw, throttle, translation float64) {
	y.controlLock.Lock()
	defer y.controlLock.Unlock()

	y.yaw = yaw
	y.throttle = throttle
	y.translation = translation
}

func (y *YawMaintainer) loop(cxt context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	y.i2cLock.Lock()
	m, err := imu.New("/dev/i2c-1")
	if err != nil {
		fmt.Println("Failed to open IMU", err)
		y.i2cLock.Unlock()
		panic("Failed to open IMU")
	}

	m.Configure()
	m.Calibrate()
	m.ResetFIFO()
	y.i2cLock.Unlock()

	ticker := time.NewTimer(50 * time.Millisecond)
	defer ticker.Stop()

	for cxt.Err() == nil {
		<-ticker.C

		y.i2cLock.Lock()
		yawReading := m.ReadGyroX()
		y.i2cLock.Unlock()

		y.controlLock.Lock()
		targetYaw := y.yaw
		targetThrottle := y.throttle
		targetTranslation := y.translation
		y.controlLock.Unlock()

		yawError := targetYaw - float64(yawReading)/32768

		// Map the values to speeds for each motor.
		frontLeft := targetThrottle + yawError + targetTranslation
		frontRight := targetThrottle - yawError - targetTranslation
		backLeft := targetThrottle + yawError - targetTranslation
		backRight := targetThrottle - yawError + targetTranslation

		m1 := math.Max(frontLeft, frontRight)
		m2 := math.Max(backLeft, backRight)
		m := math.Max(m1, m2)
		scale := 1.0
		if m > 1 {
			scale = 1.0 / m
		}

		fl := scaleAndClamp(frontLeft*scale, 127)
		fr := scaleAndClamp(frontRight*scale, 127)
		bl := scaleAndClamp(backLeft*scale, 127)
		br := scaleAndClamp(backRight*scale, 127)

		y.i2cLock.Lock()
		y.Propeller.SetMotorSpeeds(fl, fr, bl, br)
		y.i2cLock.Unlock()
	}
}
