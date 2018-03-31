package rainbowmode

import (
	"context"
	"sync"

	"fmt"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"gocv.io/x/gocv"
	"math/rand"
	"sync/atomic"
	"time"
)

type Colour int

const (
	Red Colour = iota
	Blue
	Yellow
	Green
)

func (c Colour) String() string {
	switch c {
	case Red:
		return "red"
	case Blue:
		return "blue"
	case Yellow:
		return "yellow"
	case Green:
		return "green"
	}
	return fmt.Sprintf("Colour(%d)", int(c))
}

var targetSequence = []Colour{Red, Blue, Yellow, Green}

type RainbowMode struct {
	Propeller      propeller.Interface
	cancel         context.CancelFunc
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

	running        bool
	targetBallIdx  int
	cancelSequence context.CancelFunc
	sequenceDone     chan struct{}

	paused int32
}

func New(propeller propeller.Interface) *RainbowMode {
	return &RainbowMode{
		Propeller:      propeller,
		joystickEvents: make(chan *joystick.Event),
	}
}

func (m *RainbowMode) Name() string {
	return "Rainbow mode"
}

func (m *RainbowMode) Start(ctx context.Context) {
	m.stopWG.Add(1)
	var loopCtx context.Context
	loopCtx, m.cancel = context.WithCancel(ctx)
	go m.loop(loopCtx)
}

func (m *RainbowMode) Stop() {
	m.cancel()
	m.stopWG.Wait()
}

// loop processes input and starts/stops the goroutine that does the actual CV work.
func (m *RainbowMode) loop(ctx context.Context) {
	defer m.stopWG.Done()
	defer m.stopSequence()

	for {
		select {
		case <-m.sequenceDone:
			m.running = false
			m.sequenceDone = nil
		case <-ctx.Done():
			return
		case event := <-m.joystickEvents:
			if event.Type == joystick.EventTypeButton && event.Value == 1 {
				switch event.Number {
				case joystick.ButtonR1:
					m.startSequence()
				case joystick.ButtonSquare:
					m.stopSequence()
				case joystick.ButtonTriangle:
					m.pauseOrResumeSequence()
				}
			}
		}
	}
}

func (m *RainbowMode) startSequence() {
	if m.running {
		fmt.Println("Already running")
		return
	}

	fmt.Println("Starting sequence...")
	m.targetBallIdx = 0
	m.running = true
	atomic.StoreInt32(&m.paused, 0)

	seqCtx, cancel := context.WithCancel(context.Background())
	m.cancelSequence = cancel
	m.sequenceDone = make(chan struct{})
	go m.runSequence(seqCtx)
}

// runSequence is a goroutine that reads form the camera and controls the motors.
func (m *RainbowMode) runSequence(ctx context.Context) {
	defer close(m.sequenceDone)

	webcam, err := gocv.VideoCaptureDevice(0)
	if err != nil {
		fmt.Printf("error opening video capture device: %v\n", 0)
		return
	}
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()

	startTime := time.Now()

	fmt.Println("Next target ball: ", targetSequence[m.targetBallIdx])

	defer fmt.Println("Exiting sequence loop")

	for m.targetBallIdx < len(targetSequence) && ctx.Err() == nil {
		for atomic.LoadInt32(&m.paused) == 1 && ctx.Err() == nil {
			m.Propeller.SetMotorSpeeds(0,0,0,0)
			time.Sleep(100 * time.Millisecond)
		}

		// This blocks until the next frame is ready.
		if ok := webcam.Read(img); !ok {
			fmt.Printf("cannot read device\n")
			time.Sleep(1 * time.Millisecond)
			continue
		}
		if img.Empty() {
			fmt.Printf("no image on device\n")
			time.Sleep(1 * time.Millisecond)
			continue
		}

		targetColour := targetSequence[m.targetBallIdx]

		// TODO implement ball detection and motor control :-)
		reachedTargetBall := rand.Float32() < 0.01

		if reachedTargetBall {
			fmt.Println("Reached target ball:", targetColour, "in", time.Since(startTime))
			m.targetBallIdx++
			if m.targetBallIdx < len(targetSequence) {
				fmt.Println("Next target ball: ", targetSequence[m.targetBallIdx])
			} else {
				fmt.Println("Done!!")
			}
		}
	}
}

func (m *RainbowMode) stopSequence() {
	if !m.running {
		fmt.Println("Not running")
		return
	}
	fmt.Println("Stopping sequence...")

	m.cancelSequence()
	m.cancelSequence = nil
	<-m.sequenceDone
	m.running = false
	m.sequenceDone = nil
	atomic.StoreInt32(&m.paused, 0)
}

func (m *RainbowMode) pauseOrResumeSequence() {
	if atomic.LoadInt32(&m.paused) == 1 {
		fmt.Println("Resuming sequence...")
		atomic.StoreInt32(&m.paused, 0)
	} else {
		fmt.Println("Pausing sequence...")
		atomic.StoreInt32(&m.paused, 1)
	}
}

func (m *RainbowMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}
