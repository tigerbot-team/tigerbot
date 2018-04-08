package duckshoot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
)

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
	PitchAutoRepeatInterval = 10 * time.Millisecond

	MotorStopTime = time.Second
)

type ServoController struct {
	propLock  *sync.Mutex // Guards access to the propeller
	propeller propeller.Interface

	stopC  chan struct{}
	doneWg sync.WaitGroup

	joystickEvents chan *joystick.Event
}

func NewServoController() *ServoController {
	return &ServoController{
		joystickEvents: make(chan *joystick.Event),
	}
}

func (d *ServoController) Start(propLock *sync.Mutex, propeller propeller.Interface) {
	d.propLock = propLock
	d.propeller = propeller
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
	var ballThrowerPitch uint8 = ServoValuePitchDefault

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
		d.propLock.Lock()
		d.propeller.SetServo(ServoPitch, ballThrowerPitch)
		d.propLock.Unlock()
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
		motorTop    uint8 = ServoValueMotorOff
		motorBottom uint8 = ServoValueMotorOff
		plunger     uint8 = ServoValuePlungerDefault
	)

	var motorStopTimer *time.Timer
	var motorStopC <-chan time.Time

	updateServos := func() {
		d.propLock.Lock()
		d.propeller.SetServo(ServoMotor1, motorTop)
		d.propeller.SetServo(ServoMotor2, motorBottom)
		d.propeller.SetServo(ServoPlunger, plunger)
		d.propLock.Unlock()
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
			fmt.Println("Fire control loop stopping")
			return
		}
	}
}

func (d *ServoController) OnJoystickEvent(event *joystick.Event) {
	d.joystickEvents <- event
}
