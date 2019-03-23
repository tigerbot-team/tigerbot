package nebulamode

import (
	"context"
	"image"
	"io/ioutil"
	"math"
	"sync"

	"fmt"
	"sync/atomic"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mazemode"
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
	DirectionAdjustFactor float64
	Sequence              []string
	Balls                 map[string]rainbow.HSVRange

	ForwardCornerDetectionThreshold   int
	ForwardLRCornerDetectionThreshold int
	CornerSlowDownThresh              int

	ForwardReverseThreshold int

	// The part of a still corner photo that we look at to
	// determine the colour in that corner.
	CentralRegionXPercent int
	CentralRegionYPercent int
}

type NebulaMode struct {
	hw hardware.Interface

	cancel         context.CancelFunc
	startTrigger   chan struct{}
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

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
	pictureIndex            int
	ballInView              bool

	// Config
	config RainbowConfig
}

func New(hw hardware.Interface) *NebulaMode {
	m := &NebulaMode{
		hw:             hw,
		joystickEvents: make(chan *joystick.Event),
		phase:          Rotating,
		config: RainbowConfig{
			RotateSpeed:           16,
			SlowRotateSpeed:       5,
			LimitSpeed:            80,
			ForwardSpeed:          35,
			ForwardSlowSpeed:      6,
			XStraightAhead:        320,
			DirectionAdjustFactor: 0.03,
			Sequence:              []string{"red", "blue", "yellow", "green"},
			Balls:                 map[string]rainbow.HSVRange{},

			ForwardCornerDetectionThreshold:   120,
			ForwardLRCornerDetectionThreshold: 78,
			CornerSlowDownThresh:              60,

			ForwardReverseThreshold: 450,

			// Percentages of the width and height of a
			// corner photo that we use, centred around
			// the centroid, to determine the colour in
			// that corner.
			CentralRegionXPercent: 10,
			CentralRegionYPercent: 10,
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
	return "NEBULA MODE"
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
						fmt.Println("Getting ready!")
						m.startSequence()
					case joystick.ButtonSquare:
						m.stopSequence()
					case joystick.ButtonTriangle:
						m.pauseOrResumeSequence()
					}
				} else {
					switch event.Number {
					case joystick.ButtonR1:
						fmt.Println("GO!")
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

func (m *NebulaMode) takePicture() (hsv gocv.Mat, err error) {
	webcam, werr := gocv.VideoCaptureDevice(0)
	if werr != nil {
		err = fmt.Errorf("error opening video capture device: %v", werr)
		return
	}
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()
	if ok := webcam.Read(img); !ok {
		err = fmt.Errorf("cannot read picture from webcam device")
		return
	}
	hsv = gocv.NewMat()
	gocv.CvtColor(img, hsv, gocv.ColorBGRToHSV)
	return
}

func (m *NebulaMode) fatal(err error) {
	// Placeholder for what to do if we hit a fatal error.
	// Callers assume that this does not return normally.
	panic(err)
}

func (m *NebulaMode) calculateVisitOrder(hsv [4]gocv.Mat) []int {
	averageHue := make([]byte, len(hsv))
	hueUsed := make([]bool, len(hsv))
	for ii := range hsv {
		averageHue[ii] = m.calculateAverageHue(hsv[ii])
		hueUsed[ii] = false
	}
	var targets []*rainbow.HSVRange
	for _, colour := range m.config.Sequence {
		targets = append(targets, rainbow.Balls[colour])
	}
	bestMatchCost, bestMatchOrder := m.findBestMatch(targets, averageHue, hueUsed)
	return bestMatchOrder
}

func (m *NebulaMode) calculateAverageHue(hsv gocv.Mat) byte {
	w := hsv.Cols() / 2
	h := hsv.Rows() / 2
	dw := (w * m.config.CentralRegionXPercent) / 100
	dh := (h * m.config.CentralRegionYPercent) / 100
	centralRegion := image.Rect(w-dw, h-dh, w+dw, h+dh)
	cropped := hsv.Region(centralRegion)
	mean := cropped.Mean()
	return byte(math.Round(mean.Val1))
}

func (m *NebulaMode) findBestMatch(targets []*rainbow.HSVRange, averageHue []byte, hueUsed []bool) (int, []int) {
	var (
		minCost  int
		minOrder []int
	)
	for ic, choiceHue := range averageHue {
		if hueUsed[ic] {
			continue
		}
		choiceCost := m.calculateCost(targets[0], choiceHue)
		choiceOrder := []int{ic}
		if len(targets) > 1 {
			// This is not the last target.
			hueUsedCopy := make([]bool, len(hueUsed))
			copy(hueUsedCopy, hueUsed)
			hueUsedCopy[ic] = true
			nextCost, nextOrder := m.findBestMatch(targets[1:], averageHue, hueUsedCopy)
			choiceCost = choiceCost + nextCost
			choiceOrder = append(choiceOrder, nextOrder...)
		}
		if (minOrder == nil) || (choiceCost < minCost) {
			minCost = choiceCost
			minOrder = choiceOrder
		}
	}
	return minCost, minOrder
}

func (m *NebulaMode) calculateCost(targetHSVRange *rainbow.HSVRange, choiceHue byte) int {
	var hueDelta byte = 0
	if targetHSVRange.HueMin <= targetHSVRange.HueMax {
		// Non-wrapped hue range.
		if choiceHue < targetHSVRange.HueMin {
			hueDelta = targetHSVRange.HueMin - choiceHue
		} else if choiceHue > targetHSVRange.HueMax {
			hueDelta = choiceHue - targetHSVRange.HueMax
		}
	} else {
		// Wrapped hue range.
		if (choiceHue < targetHSVRange.HueMin) && (choiceHue > targetHSVRange.HueMax) {
			delta1 := targetHSVRange.HueMin - choiceHue
			delta2 := choiceHue - targetHSVRange.HueMax
			if delta1 < delta2 {
				hueDelta = delta1
			} else {
				hueDelta = delta2
			}
		}
	}
	return int(hueDelta) * int(hueDelta)
}

// runSequence is a goroutine that reads from the camera and controls the motors.
func (m *NebulaMode) runSequence(ctx context.Context) {
	defer close(m.sequenceDone)
	defer fmt.Println("Exiting sequence loop")

	// Create time-of-flight reading filters; should filter out any stray readings.
	var filters []*mazemode.Filter
	for i := 0; i < 6; i++ {
		filters = append(filters, &mazemode.Filter{})
	}

	var readings hardware.DistanceReadings

	readSensors := func() {
		// Read the sensors
		msg := ""
		readings = m.hw.CurrentDistanceReadings(readings.Revision)
		for j, r := range readings.Readings {
			prettyPrinted := "-"
			readingInMM, err := r.DistanceMM, r.Error
			filters[j].Accumulate(readingInMM)
			if readingInMM == tofsensor.RangeTooFar {
				prettyPrinted = ">2000mm"
			} else if err != nil {
				prettyPrinted = "<failed>"
			} else {
				prettyPrinted = fmt.Sprintf("%dmm", readingInMM)
			}
			msg += fmt.Sprintf("%s=%5s/%5dmm ", filters[j].Name, prettyPrinted, filters[j].BestGuess())
		}
		fmt.Println(msg)
	}

	leftRear := filters[0]
	leftRear.Name = "LR"
	leftFore := filters[1]
	leftFore.Name = "LF"
	frontLeft := filters[2]
	frontLeft.Name = "FL"
	frontRight := filters[3]
	frontRight.Name = "FR"
	rightFore := filters[4]
	rightFore.Name = "RF"
	rightRear := filters[5]
	rightRear.Name = "RR"

	startTime := time.Now()

	// We use the absolute heading hold mode so we can do things like "turn right 90 degrees".
	hh := m.hw.StartHeadingHoldMode()

	var (
		hsv [4]gocv.Mat
		err error
	)

	// Store initial heading.  Images 0, 1, 2, 3 will be at +45,
	// +135, +225 and +315 w.r.t. this initial heading.
	initialHeading := m.hw.CurrentHeading()
	cornerHeadings := [4]float64{
		initialHeading + 45,
		initialHeading + 45 + 90,
		initialHeading + 45 + 90 + 90,
		initialHeading + 45 + 90 + 90 + 90,
	}

	// Turn to take photos of the four corners.
	for ii, cornerHeading := range cornerHeadings {
		hh.SetHeading(cornerHeading)
		_ = hh.Wait(ctx)
		hsv[ii], err = m.takePicture()
		if err != nil {
			m.fatal(err)
		}
		defer hsv[ii].Close()
	}

	// Calculate the order we need to visit the corners, by
	// matching photos to colours.
	visitOrder := m.calculateVisitOrder(hsv)

	for _, index := range visitOrder {
		// Rotating phase.
		hh.SetHeading(cornerHeadings[index])
		_ = hh.Wait(ctx)
		// Advancing phase.
		hh.SetThrottle(m.config.ForwardSpeed)
		advanceFastStart := time.Now()
		var (
			advanceFastDuration time.Duration
			advanceSlowStart    time.Time
			advanceSlowDuration time.Duration
		)
		for ctx.Err() == nil {
			readSensors()
			fmt.Println("Target:", index, "Advancing")

			closeEnough :=
				(!frontLeft.IsFar() && frontLeft.BestGuess() <= m.config.ForwardCornerDetectionThreshold) ||
					(!frontRight.IsFar() && frontRight.BestGuess() <= m.config.ForwardCornerDetectionThreshold)

			closeEnoughToSlowDown :=
				(!frontLeft.IsFar() && frontLeft.BestGuess() <= m.config.ForwardCornerDetectionThreshold+m.config.CornerSlowDownThresh) ||
					(!frontRight.IsFar() && frontRight.BestGuess() <= m.config.ForwardLRCornerDetectionThreshold+m.config.CornerSlowDownThresh)

			if closeEnough {
				advanceSlowDuration = time.Since(advanceSlowStart)
				break
			}

			// We're approaching the ball but not yet close enough.
			if closeEnoughToSlowDown {
				fmt.Println("Slowing...")
				advanceFastDuration = time.Since(advanceFastStart)
				hh.SetThrottle(m.config.ForwardSlowSpeed)
				advanceSlowStart = time.Now()
			}
		}
		// Reversing phase.
		hh.SetThrottle(-m.config.ForwardSpeed)
		reverseStart := time.Now()
		reverseDuration := time.Duration(
			((float64(advanceFastDuration) * m.config.ForwardSpeed) +
				(float64(advanceSlowDuration) * m.config.ForwardSlowSpeed)) /
				m.config.ForwardSpeed)
		for ctx.Err() == nil {
			readSensors()
			fmt.Println("Target:", index, "Reversing")

			closeEnough :=
				(!frontLeft.IsFar() && frontLeft.BestGuess() <= m.config.ForwardCornerDetectionThreshold) ||
					(!frontRight.IsFar() && frontRight.BestGuess() <= m.config.ForwardCornerDetectionThreshold)

			closeEnoughToSlowDown :=
				(!frontLeft.IsFar() && frontLeft.BestGuess() <= m.config.ForwardCornerDetectionThreshold+m.config.CornerSlowDownThresh) ||
					(!frontRight.IsFar() && frontRight.BestGuess() <= m.config.ForwardLRCornerDetectionThreshold+m.config.CornerSlowDownThresh)

			if closeEnough {
				break
			}

			// We're approaching the ball but not yet close enough.
			if closeEnoughToSlowDown {
				fmt.Println("Slowing...")
				hh.SetThrottle(m.config.ForwardSlowSpeed)
				advanceSlowStart := time.Now()
			}
		}
	}

	m.reset()
	fmt.Println("Next target ball: ", m.config.Sequence[m.targetBallIdx])

	defer fmt.Println("Exiting sequence loop")

	for m.targetBallIdx < len(m.config.Sequence) && ctx.Err() == nil {
		for atomic.LoadInt32(&m.paused) == 1 && ctx.Err() == nil {
			hh.SetThrottle(0)
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

		fmt.Println("Reached target ball:", m.targetColour, "in", time.Since(startTime))
		m.targetBallIdx++
		if m.targetBallIdx < len(m.config.Sequence) {
			fmt.Println("Next target ball: ", m.config.Sequence[m.targetBallIdx])
			m.announceTargetBall()
		} else {
			fmt.Println("Done!!")
		}
	}

	hh.SetThrottle(0)
}

func (m *NebulaMode) announceTargetBall() {
	m.hw.PlaySound(
		fmt.Sprintf("/sounds/%vball.wav", m.config.Sequence[m.targetBallIdx]),
	)
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
		fmt.Println("NEBULA: Not running")
		return
	}
	fmt.Println("NEBULA: Stopping sequence...")

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
