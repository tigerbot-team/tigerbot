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

	headingHolder *HeadingHolder
}

func New(name string, propeller propeller.Interface, servoController ServoController) *RCMode {
	r := &RCMode{
		Propeller:       propeller,
		joystickEvents:  make(chan *joystick.Event),
		servoController: servoController,
		name:            name,
	}
	r.headingHolder = &HeadingHolder{
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
	go m.headingHolder.loop(loopCtx, &m.stopWG)
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
	yaw = applyExpo(float64(lStickX)/32767.0, 2.5) / 8
	throttle = applyExpo(float64(rStickY)/-32767.0, expo) / 8
	translate = applyExpo(float64(rStickX)/32767.0, expo) / 8
	return
}

func MixAggressive(lStickX, lStickY, rStickX, rStickY int16) (yaw, throttle, translate float64) {
	const expo = 1.6
	_ = lStickY

	// Put all the values into the range (-1, 1) and apply expo.
	yaw = applyExpo(float64(lStickX)/32767.0, 2.5)
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

type HeadingHolder struct {
	i2cLock   *sync.Mutex // Guards access to the propeller
	Propeller propeller.Interface

	controlLock                sync.Mutex
	yaw, throttle, translation float64
}

func (y *HeadingHolder) SetControlInputs(yaw, throttle, translation float64) {
	y.controlLock.Lock()
	defer y.controlLock.Unlock()

	y.yaw = yaw
	y.throttle = throttle
	y.translation = translation
}

func (y *HeadingHolder) loop(cxt context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	defer fmt.Println("Heading holder loop exited")

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

	const imuDT = 10 * time.Millisecond
	const targetLoopDT = 20 * time.Millisecond

	ticker := time.NewTicker(targetLoopDT)
	defer ticker.Stop()

	var headingEstimate float64
	var targetHeading float64
	var filteredTranslation float64
	var motorRotationSpeed float64
	var lastHeadingError float64
	var iHeadingError float64

	const (
		kp                        = 0.020
		ki                        = 0.0
		kd                        = -0.00008
		maxIntegral               = 1
		maxMotorSpeed             = 2.0
		maxTranslationDeltaPerSec = 1
	)
	maxTranslationDelta := maxTranslationDeltaPerSec * targetLoopDT.Seconds()
	var lastLoopStart = time.Now()

	for cxt.Err() == nil {
		<-ticker.C
		now := time.Now()
		loopTime := now.Sub(lastLoopStart)
		lastLoopStart = now

		// Integrate the output from the IMU to get our heading estimate.
		y.i2cLock.Lock()
		yawReadings := m.ReadFIFO()
		y.i2cLock.Unlock()

		for _, yaw := range yawReadings {
			yawDegreesPerSec := float64(yaw) * m.DegreesPerLSB()
			headingEstimate -= imuDT.Seconds() * yawDegreesPerSec
		}

		// Grab the current control values.
		y.controlLock.Lock()
		targetYaw := y.yaw
		targetThrottle := y.throttle
		targetTranslation := y.translation
		y.controlLock.Unlock()

		// Update our target heading accordingly.
		loopTimeSecs := loopTime.Seconds()

		// Avoid letting the yaw lead the heading too much.
		if math.Abs(targetHeading-headingEstimate) < 40 {
			targetHeading += loopTimeSecs * targetYaw * 300
		}

		// Calculate the error/derivative/integral.
		headingError := targetHeading - headingEstimate
		dHeadingError := (headingError - lastHeadingError) / loopTimeSecs
		iHeadingError += headingError * loopTimeSecs
		if iHeadingError > maxIntegral {
			iHeadingError = maxIntegral
		} else if iHeadingError < -maxIntegral {
			iHeadingError = -maxIntegral
		}

		// Calculate the correction to apply.
		rotationCorrection := kp*headingError + ki*iHeadingError + kd*dHeadingError

		// Add the correction to the current speed.  We want 0 correction to mean "hold the same motor speed".
		motorRotationSpeed = rotationCorrection
		if motorRotationSpeed > maxMotorSpeed {
			motorRotationSpeed = maxMotorSpeed
		} else if motorRotationSpeed < -maxMotorSpeed {
			motorRotationSpeed = -maxMotorSpeed
		}

		fmt.Printf("%v Heading: %.1f Target: %.1f Error: %.1f Int: %.1f D: %.1f -> %.1f\n",
			loopTime, headingEstimate, targetHeading, headingError, iHeadingError, dHeadingError, motorRotationSpeed)

		if math.Abs(targetTranslation) < 0.2 {
			filteredTranslation = targetTranslation
		} else if targetTranslation > filteredTranslation+maxTranslationDelta {
			filteredTranslation += maxTranslationDelta
		} else if targetTranslation < filteredTranslation-maxTranslationDelta {
			filteredTranslation -= maxTranslationDelta
		} else {
			filteredTranslation = targetTranslation
		}

		// Map the values to speeds for each motor.
		frontLeft := targetThrottle + motorRotationSpeed + filteredTranslation
		frontRight := targetThrottle - motorRotationSpeed - filteredTranslation
		backLeft := targetThrottle + motorRotationSpeed - filteredTranslation
		backRight := targetThrottle - motorRotationSpeed + filteredTranslation

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

		lastHeadingError = headingError
	}
	y.i2cLock.Lock()
	y.Propeller.SetMotorSpeeds(0, 0, 0, 0)
	y.i2cLock.Unlock()

}
