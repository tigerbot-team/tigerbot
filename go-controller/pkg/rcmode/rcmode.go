package rcmode

import (
	"context"
	"math"
	"sync"

	"fmt"

	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
)

type RCMode struct {
	propLock  sync.Mutex // Guards access to the propeller
	Propeller propeller.Interface

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

const (
	ServoMotor1  = 1
	ServoMotor2  = 2
	ServoPlunger = 3
	ServoPitch   = 4

	ServoValueMotorOff = 0
	ServoValueMotorOn  = 255

	ServoValuePlungerDefault = 0
	ServoValuePlungerActive  = 255

	ServoValuePitchDefault  = 127
	ServoMaxPitch           = 255
	ServoMinPitch           = 0
	PitchAutoRepeatInterval = 50 * time.Millisecond

	MotorStopTime = time.Second
)

func (m *RCMode) loop(ctx context.Context) {
	defer m.stopWG.Done()

	var leftStickX, leftStickY, rightStickX, rightStickY int16
	var dPadY int16
	var ballThrowerPitch uint8 = 127
	var mix = MixAggressive

	// Start a goroutine to do fire-control sequencing for the ball flinger.
	fireControlCtx, cancelFireControl := context.WithCancel(context.Background())
	var fireControlDone sync.WaitGroup
	var triggerDownC = make(chan bool)
	fireControlDone.Add(1)
	go m.fireControlLoop(fireControlCtx, fireControlDone, triggerDownC)
	defer func() {
		cancelFireControl()
		fireControlDone.Wait()
		close(triggerDownC)
	}()

	// Create a ticker to do auto-repeat when the up/down button is held down.  We enable/disable auto-repeat
	// by copying the ticker's channel to autoRepeatC, or setting autoRepeatC to nil.
	autoRepeatTicker := time.NewTicker(PitchAutoRepeatInterval)
	defer autoRepeatTicker.Stop()
	var autoRepeatC <-chan time.Time

	// Function to do one iteration of updating the pitch of the ball flinger.
	updatePitch := func() {
		if dPadY > 0 {
			if ballThrowerPitch < ServoMaxPitch {
				ballThrowerPitch++ // If changing to bigger increment, be careful of wrap-around
			}
		} else if dPadY < 0 {
			if ballThrowerPitch > ServoMinPitch {
				ballThrowerPitch--
			}
		}

		fmt.Println("Setting pitch:", ballThrowerPitch)
		m.propLock.Lock()
		m.Propeller.SetServo(ServoPitch, ballThrowerPitch)
		m.propLock.Unlock()
	}

	// Set initial pitch of ball flinger.
	updatePitch()

	for {
		select {
		case <-ctx.Done():
			return
		case <-autoRepeatC:
			updatePitch()
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
				case joystick.AxisDPadY:
					dPadY = event.Value
					if dPadY != 0 {
						autoRepeatC = autoRepeatTicker.C
						select {
						case <-autoRepeatC:
						default:
						}
						updatePitch()
					} else {
						autoRepeatC = nil
					}
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
				case joystick.ButtonR2:
					if event.Value == 1 {
						fmt.Println("Trigger down!")
						triggerDownC <- true
					} else {
						fmt.Println("Trigger up!")
						triggerDownC <- false
					}
				}
			}

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

func (m *RCMode) fireControlLoop(ctx context.Context, wg sync.WaitGroup, triggerDownC chan bool) {
	defer wg.Done()

	var (
		motorTop    uint8 = ServoValueMotorOff
		motorBottom uint8 = ServoValueMotorOff
		plunger     uint8 = ServoValuePlungerDefault
	)

	var motorStopTimer *time.Timer
	var motorStopC <-chan time.Time

	updateServos := func() {
		m.propLock.Lock()
		m.Propeller.SetServo(ServoMotor1, motorTop)
		m.Propeller.SetServo(ServoMotor2, motorBottom)
		m.Propeller.SetServo(ServoPlunger, plunger)
		m.propLock.Unlock()
	}

	stopTimer := func() {
		if motorStopTimer == nil {
			return
		}
		motorStopTimer.Stop()
		motorStopC = nil
	}

	startTimer := func() {
		stopTimer()
		motorStopTimer = time.NewTimer(MotorStopTime)
		motorStopC = motorStopTimer.C
	}

	defer func() {
		stopTimer()
		motorTop = ServoValueMotorOff
		motorBottom = ServoValueMotorOff
		plunger = ServoValuePlungerDefault
		updateServos()
	}()

	for {
		updateServos()
		select {
		case triggerDown := <-triggerDownC:
			if triggerDown {
				// Trigger down, start motors; retract plunger to allow ball into channel.
				fmt.Println("Trigger down, activating plunger and motors")
				plunger = ServoValuePlungerActive
				motorTop = ServoValueMotorOn
				motorBottom = ServoValueMotorOn
				stopTimer()
			} else {
				// Trigger up, push plunger forward to push ball into motors.  Start the motor shutoff timer.
				fmt.Println("Trigger up, returning plunger to default position")
				plunger = ServoValuePlungerDefault
				startTimer()
			}
		case <-motorStopC:
			fmt.Println("Motor shutdown timer popped")
			motorTop = ServoValueMotorOff
			motorBottom = ServoValueMotorOff
			stopTimer()
		case <-ctx.Done():
			return
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
