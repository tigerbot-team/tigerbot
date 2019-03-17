package nebulamode

import (
	"context"
	"io/ioutil"
	"math"
	"sync"

	"fmt"
	"sync/atomic"
	"time"

	"sort"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rainbow"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
	"gocv.io/x/gocv"
	yaml "gopkg.in/yaml.v2"
)

type Phase int

const (
	Rotating Phase = iota
	Advancing
	Reversing
)

type RainbowConfig struct {
	RotateSpeed           float64
	SlowRotateSpeed       float64
	LimitSpeed            float64
	ForwardSpeed          float64
	ForwardSlowSpeed      float64
	XStraightAhead        int
	XPlusOrMinus          int
	DirectionAdjustFactor float64
	CloseEnoughSize       int
	Sequence              []string
	Balls                 map[string]rainbow.HSVRange

	ForwardCornerDetectionThreshold   int
	ForwardLRCornerDetectionThreshold int
	CornerSlowDownThresh              int

	ForwardReverseThreshold int

	// Height offset in mm from the camera centreline to the lowest reasonable ball centre
	// height; positive/negative if that ball centre height is above/below the camera height.
	DeltaHMinMM int
	// Height offset in mm from the camera centreline to the highest reasonable ball centre
	// height; positive/negative if that ball centre height is above/below the camera height.
	DeltaHMaxMM int
	// Vertical field of view factor as tan(theta), where theta is the angle that the camera
	// captures (in 640x480 video mode) above and below its centreline.
	TanTheta float64
	// Whether to filter out apparent 'balls' that aren't in the expected Y range.
	FilterYCoord bool
}

type NebulaMode struct {
	Propeller      propeller.Interface
	cancel         context.CancelFunc
	startTrigger   chan struct{}
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event
	soundsToPlay   chan string

	running        bool
	targetBallIdx  int
	cancelSequence context.CancelFunc
	sequenceDone   chan struct{}

	paused int32

	// State of balls searching.
	phase                   Phase
	targetColour            string
	ballX                   int
	roughDirectionCount     int
	ballFixed               bool
	advanceReverseStartTime time.Time
	advanceDuration         time.Duration
	savePicture             int32
	pictureIndex            int
	ballInView              bool

	// Config
	config RainbowConfig
}

func New(hw *hardware.Interface, soundsToPlay chan string) *NebulaMode {
	m := &NebulaMode{
		Propeller:      hw.Motors,
		soundsToPlay:   soundsToPlay,
		joystickEvents: make(chan *joystick.Event),
		phase:          Rotating,
		config: RainbowConfig{
			RotateSpeed:           16,
			SlowRotateSpeed:       5,
			LimitSpeed:            80,
			ForwardSpeed:          35,
			ForwardSlowSpeed:      6,
			XStraightAhead:        320,
			XPlusOrMinus:          80,
			DirectionAdjustFactor: 0.03,
			CloseEnoughSize:       150,
			Sequence:              []string{"red", "blue", "yellow", "green"},
			Balls:                 map[string]rainbow.HSVRange{},

			ForwardCornerDetectionThreshold:   120,
			ForwardLRCornerDetectionThreshold: 78,
			CornerSlowDownThresh:              60,

			ForwardReverseThreshold: 450,

			DeltaHMinMM: -28,
			DeltaHMaxMM: 17,
			// tan(24.4 degrees) * 480 / 1232, following
			// https://www.raspberrypi.org/documentation/hardware/camera/README.md and
			// https://github.com/waveform80/picamera/blob/master/docs/fov.rst
			TanTheta:     0.177,
			FilterYCoord: false,
		},
		startTrigger: make(chan struct{}),
	}
	for _, colour := range m.config.Sequence {
		m.config.Balls[colour] = *rainbow.Balls[colour]
	}
	cfg, err := ioutil.ReadFile("/cfg/rainbow.yaml")
	if err != nil {
		fmt.Println(err)
	} else {
		err = yaml.Unmarshal(cfg, &m.config)
		if err != nil {
			fmt.Println(err)
		}
	}
	// Write out the config that we are using.
	fmt.Printf("Using config: %#v\n", m.config)
	cfgBytes, err := yaml.Marshal(&m.config)
	//fmt.Printf("Marshalled: %#v\n", cfgBytes)
	if err != nil {
		fmt.Println(err)
	} else {
		err = ioutil.WriteFile("/cfg/rainbow-in-use.yaml", cfgBytes, 0666)
		if err != nil {
			fmt.Println(err)
		}
	}
	return m
}

func (m *NebulaMode) Name() string {
	return "Rainbow mode"
}

func (m *NebulaMode) StartupSound() string {
	return "/sounds/nebulamode.wav"
}

func (m *NebulaMode) Start(ctx context.Context) {
	m.stopWG.Add(1)
	var loopCtx context.Context
	loopCtx, m.cancel = context.WithCancel(ctx)
	go m.loop(loopCtx)
}

func (m *NebulaMode) Stop() {
	m.cancel()
	m.stopWG.Wait()
}

// loop processes input and starts/stops the goroutine that does the actual CV work.
func (m *NebulaMode) loop(ctx context.Context) {
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
			switch event.Type {
			case joystick.EventTypeButton:
				if event.Value == 1 {
					switch event.Number {
					case joystick.ButtonR1:
						m.startSequence()
					case joystick.ButtonSquare:
						m.stopSequence()
					case joystick.ButtonTriangle:
						m.pauseOrResumeSequence()
					case joystick.ButtonCircle:
						atomic.StoreInt32(&m.savePicture, 1)
					}
				} else {
					switch event.Number {
					case joystick.ButtonR1:
						close(m.startTrigger)
						m.announceTargetBall()
					}
				}
			}
		}
	}
}

func (m *NebulaMode) startSequence() {
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

func clamp(v float64, limit float64) int8 {
	if v < -limit {
		v = -limit
	}
	if v > limit {
		v = limit
	}
	if v <= math.MinInt8 {
		return math.MinInt8
	}
	if v >= math.MaxInt8 {
		return math.MaxInt8
	}
	return int8(v)
}

func (m *NebulaMode) setSpeeds(forwards, sideways, rotation float64) {
	fl := clamp(forwards-sideways-rotation, m.config.LimitSpeed)
	fr := clamp(forwards+sideways+rotation, m.config.LimitSpeed)
	bl := clamp(forwards+sideways-rotation, m.config.LimitSpeed)
	br := clamp(forwards-sideways+rotation, m.config.LimitSpeed)

	fmt.Printf("Speeds: FL=%d FR=%d BL=%d BR=%d\n", fl, fr, bl, br)
	m.Propeller.SetMotorSpeeds(fl, fr)
}

// runSequence is a goroutine that reads form the camera and controls the motors.
func (m *NebulaMode) runSequence(ctx context.Context) {
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
		mux.BusTOFSideLeft,
		mux.BusTOFFrontLeft,
		mux.BusTOFForward,
		mux.BusTOFFrontRight,
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

	var filters []*Filter
	for i := 0; i < 5; i++ {
		filters = append(filters, &Filter{})
	}

	readSensors := func() {
		// Read the sensors
		for j, tof := range tofs {
			reading := "-"
			readingInMM, err := tof.GetNextContinuousMeasurement()
			filters[j].Accumulate(readingInMM)
			if readingInMM == tofsensor.RangeTooFar {
				reading = ">2000mm"
			} else if err != nil {
				reading = "<failed>"
			} else {
				reading = fmt.Sprintf("%dmm", readingInMM)
			}
			fmt.Printf("%s:=%5s/%5dmm ", filters[j].Name, reading, filters[j].BestGuess())
		}
		fmt.Println()
	}

	sideLeft := filters[0]
	sideLeft.Name = "L"
	forwardLeft := filters[1]
	forwardLeft.Name = "FL"
	forward := filters[2]
	forward.Name = "F"
	forwardRight := filters[3]
	forwardRight.Name = "FR"
	sideRight := filters[4]
	sideRight.Name = "R"

	for i := 0; i < 5; i++ {
		readSensors()
	}

	webcam, err := gocv.VideoCaptureDevice(0)
	if err != nil {
		fmt.Printf("error opening video capture device: %v\n", 0)
		return
	}
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()

	startTime := time.Now()

	m.reset()
	fmt.Println("Next target ball: ", m.config.Sequence[m.targetBallIdx])

	defer fmt.Println("Exiting sequence loop")

	var (
		lastFrameTime time.Time
		numFramesRead int
	)

	for m.targetBallIdx < len(m.config.Sequence) && ctx.Err() == nil {
		for atomic.LoadInt32(&m.paused) == 1 && ctx.Err() == nil {
			m.Propeller.SetMotorSpeeds(0, 0)
			time.Sleep(100 * time.Millisecond)
		}

		m.targetColour = m.config.Sequence[m.targetBallIdx]

		select {
		case <-m.startTrigger:
			// Do rest of loop: moving etc.
		default:
			// Don't move yet.
			continue
		}

		readSensors()

		if m.phase == Reversing {
			fmt.Println("Target:", m.targetColour, "Reversing")

			farEnough := forward.BestGuess() > m.config.ForwardReverseThreshold ||
				forwardRight.BestGuess() > m.config.ForwardReverseThreshold ||
				forwardLeft.BestGuess() > m.config.ForwardReverseThreshold

			if farEnough {
				fmt.Println("Far enough away from wall, stopping reversing")
			}
			if !farEnough && time.Since(m.advanceReverseStartTime) < m.advanceDuration {
				continue
			}
			m.reset()
			m.phase = Rotating
			// Fall through.
		}

		if m.phase == Rotating {
			fmt.Println("Target:", m.targetColour, "Rotating")
			if !m.roughDirectionKnown() {
				// Continue rotating.
				if m.ballInView {
					m.setSpeeds(0, 0, m.config.SlowRotateSpeed)
				} else {
					m.setSpeeds(0, 0, m.config.RotateSpeed)
				}
				continue
			}
			m.phase = Advancing
			m.advanceReverseStartTime = time.Now()
			// Fall through.
		}

		if m.phase == Advancing {
			fmt.Println("Target:", m.targetColour, "Advancing")

			closeEnough := !forward.IsFar() && forward.BestGuess() <= m.config.ForwardCornerDetectionThreshold &&
				!forwardRight.IsFar() && forwardRight.BestGuess() <= m.config.ForwardLRCornerDetectionThreshold &&
				!forwardLeft.IsFar() && forwardLeft.BestGuess() <= m.config.ForwardLRCornerDetectionThreshold

			closeEnoughToSlowDown := !forward.IsFar() && forward.BestGuess() <= m.config.ForwardCornerDetectionThreshold+m.config.CornerSlowDownThresh ||
				!forwardRight.IsFar() && forwardRight.BestGuess() <= m.config.ForwardLRCornerDetectionThreshold+m.config.CornerSlowDownThresh ||
				!forwardLeft.IsFar() && forwardLeft.BestGuess() <= m.config.ForwardLRCornerDetectionThreshold+m.config.CornerSlowDownThresh

			if m.ballFixed && !closeEnough {
				// We're approaching the ball but not yet close enough.
				sideways := m.getTOFDifference()
				rotation := m.getDirectionAdjust()
				if closeEnoughToSlowDown {
					fmt.Println("Slowing...")
					m.setSpeeds(m.config.ForwardSlowSpeed, sideways, rotation)
				} else {
					m.setSpeeds(m.config.ForwardSpeed, sideways, rotation)
				}
				continue
			} else {
				// Either we've lost the ball, or we're close enough, so we should
				// switch to reversing.
				m.phase = Reversing
				m.advanceDuration = time.Since(m.advanceReverseStartTime)
				m.advanceReverseStartTime = time.Now()
				m.setSpeeds(-m.config.ForwardSpeed, 0, 0)
				if !m.ballFixed {
					// We haven't found the current ball yet.
					continue
				}
				// Fall through (=> we're close enough).
			}
		}

		fmt.Println("Reached target ball:", m.targetColour, "in", time.Since(startTime))
		m.targetBallIdx++
		if m.targetBallIdx < len(m.config.Sequence) {
			fmt.Println("Next target ball: ", m.config.Sequence[m.targetBallIdx])
			m.announceTargetBall()
		} else {
			fmt.Println("Done!!")
		}
	}

	m.Propeller.SetMotorSpeeds(0, 0)
}

func (m *NebulaMode) announceTargetBall() {
	m.soundsToPlay <- fmt.Sprintf("/sounds/%vball.wav", m.config.Sequence[m.targetBallIdx])
}

func (m *NebulaMode) reset() {
	m.ballX = 0
	m.roughDirectionCount = 0
	m.phase = Rotating
	m.ballFixed = false
}

func (m *NebulaMode) getDirectionAdjust() float64 {
	return m.config.DirectionAdjustFactor * float64(m.config.XStraightAhead-m.ballX)
}

func (m *NebulaMode) getTOFDifference() float64 {
	return float64(0)
}

func (m *NebulaMode) stopSequence() {
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

func (m *NebulaMode) pauseOrResumeSequence() {
	if atomic.LoadInt32(&m.paused) == 1 {
		fmt.Println("Resuming sequence...")
		atomic.StoreInt32(&m.paused, 0)
	} else {
		fmt.Println("Pausing sequence...")
		atomic.StoreInt32(&m.paused, 1)
	}
}

func (m *NebulaMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}

type Filter struct {
	Name    string
	samples []int
}

func (f *Filter) Accumulate(sample int) {
	f.samples = append(f.samples, sample)
	if len(f.samples) > 3 {
		f.samples = f.samples[1:]
	}
}

func (f *Filter) IsFar() bool {
	// Look backwards in the samples
	for i := len(f.samples) - 1; i >= 0; i-- {
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

func (f *Filter) IsGood() bool {
	return f.BestGuess() > 0 && f.BestGuess() < 200
}

func (f *Filter) BestGuess() int {
	var goodSamples []int
	for _, s := range f.samples {
		if s != 0 && s < tofsensor.RangeTooFar {
			goodSamples = append(goodSamples, s)
		}
	}
	if len(goodSamples) == 0 {
		return 0
	}
	sort.Ints(goodSamples)
	return goodSamples[len(goodSamples)/2]
}
