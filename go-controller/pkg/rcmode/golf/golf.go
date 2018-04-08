package golf

import (
	"fmt"
	"sync"
	"time"

	"math"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
)

const (
	ServoLeft  = 3
	ServoRight = 4

	ServoValuePitchDefault = 127

	ServoMaxPitch = 255
	ServoMinPitch = 0

	PitchAutoRepeatInterval = 20 * time.Millisecond
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
	var armPitch uint8 = ServoValuePitchDefault

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
			if dPadY > 0 {
				if armPitch < ServoMaxPitch {
					armPitch++
				}
			} else if dPadY < 0 {
				if armPitch > ServoMinPitch {
					armPitch--
				}
			}
		}

		fmt.Println("Setting pitch:", armPitch)
		d.propLock.Lock()
		d.propeller.SetServo(ServoLeft, armPitch)
		d.propeller.SetServo(ServoRight, math.MaxUint8-armPitch)
		d.propLock.Unlock()

		if time.Since(autoRepeatStart) > 250*time.Millisecond {
			autoRepeatFactor += 1
			if autoRepeatFactor > 15 {
				autoRepeatFactor = 15
			}
		}
	}

	// Set initial pitch of arm.
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
					autoRepeatFactor = 5
					autoRepeatStart = time.Now()
				}
			}
		}
	}
}

func (d *ServoController) OnJoystickEvent(event *joystick.Event) {
	d.joystickEvents <- event
}
