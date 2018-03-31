package mazemode

import (
	"context"
	"math"
	"sync"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"time"
	"fmt"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
	"sync/atomic"
	"sort"
	"log"
)

type MazeMode struct {
	Propeller      propeller.Interface
	cancel         context.CancelFunc
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

	running        bool
	cancelSequence context.CancelFunc
	sequenceDone     chan struct{}

	paused int32
}

func New(propeller propeller.Interface) *MazeMode {
	return &MazeMode{
		Propeller:      propeller,
		joystickEvents: make(chan *joystick.Event),
	}
}

func (m *MazeMode) Name() string {
	return "Maze mode"
}

func (m *MazeMode) Start(ctx context.Context) {
	m.stopWG.Add(1)
	var loopCtx context.Context
	loopCtx, m.cancel = context.WithCancel(ctx)
	go m.loop(loopCtx)
}

func (m *MazeMode) Stop() {
	m.cancel()
	m.stopWG.Wait()
}

func (m *MazeMode) loop(ctx context.Context) {
	defer m.stopWG.Done()



	for {
		select {
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


func (m *MazeMode) startSequence() {
	if m.running {
		fmt.Println("Already running")
		return
	}

	fmt.Println("Starting sequence...")
	m.running = true
	atomic.StoreInt32(&m.paused, 0)

	seqCtx, cancel := context.WithCancel(context.Background())
	m.cancelSequence = cancel
	m.sequenceDone = make(chan struct{})
	go m.runSequence(seqCtx)
}

func (m *MazeMode) runSequence(ctx context.Context) {
	defer close(m.sequenceDone)


	mx, err := mux.New("/dev/i2c-1")
	if err != nil {
		fmt.Println("Failed to open mux", err)
		return
	}

	var tofs []tofsensor.Interface
	defer func() {
		for _, tof := range tofs {
			tof.Close()
		}
	}()
	for _, port := range []int{
		mux.BusTOFFrontLeft,
		mux.BusTOFForward,
		mux.BusTOFFrontRight,
		mux.BusTOFSideLeft,
		mux.BusTOFSideRight,
	} {
		tof, err := tofsensor.NewMuxed("/dev/i2c-1", 0x29, mx, port)
		if err != nil {
			fmt.Println("Failed to open sensor", err)
			return
		}
		err = tof.StartContinuousMeasurements()
		if err != nil {
			fmt.Println("Failed to start continuous measurements", err)
			return
		}
		tofs = append(tofs, tof)
	}


	defer fmt.Println("Exiting sequence loop")

	const (
		wallSeparation = 350
		botWidth = 200
		sensorInset = 20
		clearance = (wallSeparation - botWidth) / 2
		targetDiagonalDistance = (clearance + sensorInset) * 1.414 /* sensor at 45 degrees */

		baseSpeed = 12
	)

	var filters []*Filter
	for i:=0; i < 5; i++ {
		filters = append(filters, &Filter{})
	}

	readSensors := func() {
		// Read the sensors
		for j, tof := range tofs {
			reading := "-"
			readingInMM, err := tof.GetNextContinuousMeasurement()
			filters[j].Accumulate(readingInMM)
			if err == tofsensor.ErrMeasurementInvalid {
				reading = "<invalid>"
			} else if err != nil {
				reading = "<failed>"
			} else {
				reading = fmt.Sprintf("%dmm", readingInMM)
			}
			fmt.Printf("%d: %5s / %5dmm ", j, reading, filters[j].BestGuess())
		}
		fmt.Println()
	}

	forwardLeft := filters[0]
	forward := filters[1]
	forwardRight := filters[2]
	sideLeft := filters[3]
	sideRight := filters[4]
	_ = forwardRight
	_ = forwardLeft
	_ = sideLeft
	_ = sideRight

	readSensors()
	readSensors()
	readSensors()
	readSensors()
	readSensors()

	for ctx.Err() == nil {
		fmt.Println("Following right wall...")
		for ctx.Err() == nil {
			m.sleepIfPaused(ctx)

			readSensors()

			targetright := targetDiagonalDistance + 10

			// If we reach a wall in front, break out and do the turn.
			const earlyTurnDist = 250
			if !forward.IsFar() {
				forwardGuess := forward.BestGuess()
				if forwardGuess < 130 {
					log.Println("Reached wall in front")
					break
				} else if forwardGuess < earlyTurnDist && forwardRight.BestGuess() < 50 {
					log.Println("right too close to wall, beginning turn...")
					break
				} else if forwardGuess < earlyTurnDist {
					// Start to turn early.
					delta := 50 + float64(earlyTurnDist-forward.BestGuess())
					targetright += delta
					fmt.Println("Early turn active", delta, "->", targetright, "mm")
				}
			}

			if forwardRight.BestGuess() == 0 {
				log.Println("Lost right wall!")
			}

			// Otherwise, try to keep the right sensor the right distance from the wall.
			errorInMM := float64(forwardRight.BestGuess()) - targetright
			errorInMMSq := math.Copysign(errorInMM*errorInMM, errorInMM)
			clamped := math.Min(math.Max(-baseSpeed*0.6, errorInMMSq*0.05), baseSpeed*0.6)

			rSpeed := int8(baseSpeed - clamped)
			lSpeed := int8(baseSpeed + clamped/2)
			rRearSpeed := int8(baseSpeed - clamped)
			lRearSpeed := int8(baseSpeed + clamped/2)

			m.Propeller.SetMotorSpeeds(lSpeed, rSpeed, lRearSpeed, rRearSpeed)
		}

		fmt.Println("Turning left...")
		m.Propeller.SetMotorSpeeds(-7, 14, -5, 9)
		for ctx.Err() == nil {
			m.sleepIfPaused(ctx)
			readSensors()
			if forward.IsFar() || forward.BestGuess() > 180 && float64(forwardRight.BestGuess()) > targetDiagonalDistance * 0.5 {
				break
			}
		}
	}

	m.Propeller.SetMotorSpeeds(0, 0, 0, 0)
}

type Filter struct {
	samples []int
}

func (f *Filter) Accumulate(sample int) {
	f.samples = append(f.samples, sample)
	if len(f.samples) > 5 {
		f.samples = f.samples[1:]
	}
}

func (f *Filter) IsFar() bool {
	// Look backwards in the samples
	for i := len(f.samples) -1; i>=0; i-- {
		if f.samples[i] > 400 {
			// 400mm is far by definition.
			return true
		}
		if f.samples[i] > 0 {
			// any recent non-far sample means we're not far.
			return false
		}
	}
	return true
}

func (f *Filter) BestGuess() int {
	var goodSamples []int
	for _, s := range f.samples {
		if s != 0 {
			goodSamples = append(goodSamples, s)
		}
	}
	if len(goodSamples) == 0 {
		return 0
	}
	sort.Ints(goodSamples)
	return goodSamples[len(goodSamples) /2]
}

func (m *MazeMode) sleepIfPaused(ctx context.Context) {
	for atomic.LoadInt32(&m.paused) == 1 && ctx.Err() == nil {
		time.Sleep(100 * time.Millisecond)
	}
}

func (m *MazeMode) stopSequence() {
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

func (m *MazeMode) pauseOrResumeSequence() {
	if atomic.LoadInt32(&m.paused) == 1 {
		fmt.Println("Resuming sequence...")
		atomic.StoreInt32(&m.paused, 0)
	} else {
		fmt.Println("Pausing sequence...")
		atomic.StoreInt32(&m.paused, 1)
	}
}

func (m *MazeMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}

