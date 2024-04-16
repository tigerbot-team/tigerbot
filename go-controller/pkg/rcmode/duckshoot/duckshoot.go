package duckshoot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcmode/servo"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
)

const (
	ServoMotor1  = 15
	ServoMotor2  = 14
	ServoPitch   = 12
	ServoPlunger = 13

	ServoValueMotorOff = 0.0
	ServoValueMotorOn  = 0.5

	ServoValuePlungerDefault = 1.0
	ServoValuePlungerActive  = 0.0

	ServoValuePitchDefault  = 0.65
	ServoMaxPitch           = 0.9
	ServoMinPitch           = 0.5
	PitchAutoRepeatInterval = 40 * time.Millisecond

	MotorStopTime = 500 * time.Millisecond
)

type ServoController struct {
	servoSetter servo.ServoSetter

	stopC  chan struct{}
	doneWg sync.WaitGroup

	joystickEvents chan *joystick.Event
}

func NewServoController() *ServoController {
	return &ServoController{
		joystickEvents: make(chan *joystick.Event),
	}
}

func (d *ServoController) Start(servoSetter servo.ServoSetter) {
	d.servoSetter = servoSetter
	d.stopC = make(chan struct{})
	d.doneWg.Add(1)

	go d.loop()
}

func (d *ServoController) Stop() {
	close(d.stopC)
	d.doneWg.Wait()
}

func (d *ServoController) loop() {
	defer d.doneWg.Done()

	fmt.Println("ServoController loop started")

	var dPadY int16
	var ballThrowerPitch float64 = ServoValuePitchDefault

	// Start a goroutine to do fire-control sequencing for the ball flinger.
	fireControlCtx, cancelFireControl := context.WithCancel(context.Background())
	var fireControlDone sync.WaitGroup
	var triggerDownC = make(chan bool)
	fireControlDone.Add(1)
	go d.fireControlLoop(fireControlCtx, &fireControlDone, triggerDownC)
	defer func() {
		fmt.Println("Cancelling fire control loop")
		cancelFireControl()
		fireControlDone.Wait()
		close(triggerDownC)
		fmt.Println("Fire control loop stopped")
	}()

	// Create a ticker to do auto-repeat when the up/down button is held down.  We enable/disable auto-repeat
	// by copying the ticker's channel to autoRepeatC, or setting autoRepeatC to nil.
	autoRepeatTicker := time.NewTicker(PitchAutoRepeatInterval)
	defer autoRepeatTicker.Stop()
	var autoRepeatC <-chan time.Time

	// Function to do one iteration of updating the pitch of the ball flinger.
	var autoRepeatStart time.Time
	var autoRepeatFactor int
	updatePitch := func() {
		for i := 0; i < autoRepeatFactor; i++ {
			if dPadY < 0 {
				if ballThrowerPitch < ServoMaxPitch {
					ballThrowerPitch += 0.01
				}
			} else if dPadY > 0 {
				if ballThrowerPitch > ServoMinPitch {
					ballThrowerPitch -= 0.01
				}
			}
		}

		fmt.Println("Setting pitch:", ballThrowerPitch)
		d.servoSetter.SetServo(ServoPitch, ballThrowerPitch)

		if time.Since(autoRepeatStart) > 250*time.Millisecond {
			autoRepeatFactor += 1
			if autoRepeatFactor > 20 {
				autoRepeatFactor = 20
			}
		}
	}

	// Set initial pitch of ball flinger.
	updatePitch()

	for {
		select {
		case <-d.stopC:
			fmt.Println("ServoController loop stopping")
			return
		case <-autoRepeatC:
			updatePitch()
		case event := <-d.joystickEvents:
			switch event.Type {
			case joystick.EventTypeAxis:
				switch event.Number {
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
					autoRepeatFactor = 1
					autoRepeatStart = time.Now()
				}
			case joystick.EventTypeButton:
				switch event.Number {
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
		}
	}
}

func (d *ServoController) fireControlLoop(ctx context.Context, wg *sync.WaitGroup, triggerDownC chan bool) {
	defer func() {
		fmt.Println("Fire control loop done")
		wg.Done()
		fmt.Println("Fire control loop done reported")
	}()

	var (
		motorTop    = ServoValueMotorOff
		motorBottom = ServoValueMotorOff
		plunger     = ServoValuePlungerDefault
	)

	var motorStopTimer *time.Timer
	var motorStopC <-chan time.Time

	updateServos := func() {
		d.servoSetter.SetServo(ServoMotor1, motorTop)
		d.servoSetter.SetServo(ServoMotor2, motorBottom)
		d.servoSetter.SetServo(ServoPlunger, plunger)
	}

	stopTimer := func() {
		if motorStopTimer == nil {
			return
		}
		fmt.Println("Stopping motor stop timer")
		motorStopTimer.Stop()
		motorStopC = nil
		fmt.Println("Stopped motor stop timer")
	}

	startTimer := func() {
		stopTimer()
		motorStopTimer = time.NewTimer(MotorStopTime)
		motorStopC = motorStopTimer.C
	}

	defer func() {
		stopTimer()

		fmt.Println("Resetting servos")
		motorTop = ServoValueMotorOff
		motorBottom = ServoValueMotorOff
		plunger = ServoValuePlungerDefault
		updateServos()
		fmt.Println("Done resetting servos")
	}()

	for {
		updateServos()
		select {
		case triggerDown := <-triggerDownC:
			if triggerDown {
				// Trigger down, start motors.
				fmt.Println("Trigger down, activating motors")
				motorTop = ServoValueMotorOn
				motorBottom = ServoValueMotorOn
				stopTimer()
			} else {
				// Trigger up, push plunger forward to push ball into motors.  Start the motor shutoff timer.
				fmt.Println("Trigger up, activating plunger to push dart forward")
				plunger = ServoValuePlungerActive
				startTimer()
			}
		case <-motorStopC:
			fmt.Println("Motor shutdown timer popped, reset plunger")
			motorTop = ServoValueMotorOff
			motorBottom = ServoValueMotorOff
			plunger = ServoValuePlungerDefault
			stopTimer()
		case <-ctx.Done():
			fmt.Println("Fire control loop stopping")
			return
		}
	}
}

func (d *ServoController) OnJoystickEvent(event *joystick.Event) {
	d.joystickEvents <- event
}
