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

type NebulaConfig struct {
	ForwardSpeed     float64
	ForwardSlowSpeed float64
	Sequence         []string
	Balls            map[string]rainbow.HSVRange

	ForwardCornerDetectionThreshold int
	CornerSlowDownThresh            int

	// The part of a still corner photo that we look at to
	// determine the colour in that corner.
	CentralRegionXPercent     int
	CentralRegionYPercent     int
	LeftRightPositions        int
	ValPenaltyPerHueDeviation float64
}

type NebulaMode struct {
	hw hardware.Interface

	cancel         context.CancelFunc
	startWG        sync.WaitGroup
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

	running        bool
	cancelSequence context.CancelFunc
	sequenceWG     sync.WaitGroup

	paused int32

	pictureIndex int

	visitOrder []int

	// Config
	config NebulaConfig
}

func New(hw hardware.Interface) *NebulaMode {
	m := &NebulaMode{
		hw:             hw,
		joystickEvents: make(chan *joystick.Event),
		config: NebulaConfig{
			ForwardSpeed:     0.2,
			ForwardSlowSpeed: 0.1,
			Sequence:         []string{"red", "blue", "yellow", "green"},
			Balls:            map[string]rainbow.HSVRange{},

			ForwardCornerDetectionThreshold: 150,
			CornerSlowDownThresh:            100,

			// Percentages of the width and height of a
			// corner photo that we use, centred around
			// the centroid, to determine the colour in
			// that corner.
			CentralRegionXPercent:     15,
			CentralRegionYPercent:     15,
			LeftRightPositions:        12,
			ValPenaltyPerHueDeviation: 1,
		},
	}
	for _, colour := range m.config.Sequence {
		m.config.Balls[colour] = *rainbow.Balls[colour]
	}
	cfg, err := ioutil.ReadFile("/cfg/nebula.yaml")
	if err != nil {
		fmt.Println(err)
	} else {
		err = yaml.Unmarshal(cfg, &m.config)
		if err != nil {
			fmt.Println(err)
		}
	}
	// Write out the config that we are using.
	fmt.Printf("NEBULA: Using config: %#v\n", m.config)
	cfgBytes, err := yaml.Marshal(&m.config)
	//fmt.Printf("NEBULA: Marshalled: %#v\n", cfgBytes)
	if err != nil {
		fmt.Println(err)
	} else {
		err = ioutil.WriteFile("/cfg/nebula-in-use.yaml", cfgBytes, 0666)
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
		case <-ctx.Done():
			return
		case event := <-m.joystickEvents:
			switch event.Type {
			case joystick.EventTypeButton:
				if event.Value == 1 {
					switch event.Number {
					case joystick.ButtonR1:
						fmt.Println("NEBULA: Getting ready!")
						m.startWG.Add(1)
						m.startSequence()
					case joystick.ButtonSquare:
						m.stopSequence()
					case joystick.ButtonTriangle:
						m.pauseOrResumeSequence()
					}
				} else {
					switch event.Number {
					case joystick.ButtonR1:
						fmt.Println("NEBULA: GO!")
						m.startWG.Done()
					}
				}
			}
		}
	}
}

func (m *NebulaMode) startSequence() {
	if m.running {
		fmt.Println("NEBULA: Already running")
		return
	}

	fmt.Println("NEBULA: Starting sequence...")
	m.running = true
	atomic.StoreInt32(&m.paused, 0)

	seqCtx, cancel := context.WithCancel(context.Background())
	m.cancelSequence = cancel
	m.sequenceWG.Add(1)
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
	if img.Empty() {
		err = fmt.Errorf("no image on device")
		return
	}
	fmt.Printf("NEBULA: Read image %v x %v\n", img.Cols(), img.Rows())
	m.savePicture(img)
	hsv = gocv.NewMat()
	gocv.CvtColor(img, hsv, gocv.ColorBGRToHSV)
	return
}

func (m *NebulaMode) savePicture(img gocv.Mat) {
	m.pictureIndex++
	saveFile := fmt.Sprintf("/tmp/nebula-image-%v.jpg", m.pictureIndex)
	success := gocv.IMWrite(saveFile, img)
	fmt.Printf("NEBULA: wrote %v? %v\n", saveFile, success)
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
	_, bestMatchOrder := m.findBestMatch(targets, averageHue, hueUsed)
	return bestMatchOrder
}

func (m *NebulaMode) calculateAverageHue(hsv gocv.Mat) byte {
	w := hsv.Cols() / 2
	h := hsv.Rows() / 2
	dw := (w * m.config.CentralRegionXPercent) / 100
	dh := (h * m.config.CentralRegionYPercent) / 100

	var bestScore float64
	var bestAverageHue byte
	for j := 0; j <= m.config.LeftRightPositions; j++ {
		x := ((2*w - 2*dw) * j) / m.config.LeftRightPositions
		centralRegion := image.Rect(x, h-dh, x+2*dw, h+dh)
		cropped := hsv.Region(centralRegion)
		mean := gocv.NewMat()
		stdDev := gocv.NewMat()
		gocv.MeanStdDev(cropped, &mean, &stdDev)
		//fmt.Printf("mean = %v %v\n", mean.Size(), mean.Type())
		//fmt.Printf("stdDev = %v %v\n", stdDev.Size(), stdDev.Type())
		averageHue := mean.GetDoubleAt(0, 0)
		averageSat := mean.GetDoubleAt(1, 0)
		averageVal := mean.GetDoubleAt(2, 0)
		stdDevHue := stdDev.GetDoubleAt(0, 0)
		score := averageVal - m.config.ValPenaltyPerHueDeviation*stdDevHue
		fmt.Printf("%.3v %.3v %.3v %.3v %.3v\n", averageHue, averageSat, averageVal, stdDevHue, score)
		if (j == 0) || (score > bestScore) {
			bestScore = score
			bestAverageHue = byte(math.Round(averageHue))
		}
	}

	return bestAverageHue
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
	defer m.sequenceWG.Done()
	defer fmt.Println("NEBULA: Exiting sequence loop")
	defer m.hw.StopMotorControl()

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
			filters[j].Accumulate(readingInMM, readings.CaptureTime)
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

	// We use the absolute heading hold mode so we can do things like "turn right 90 degrees".
	hh := m.hw.StartHeadingHoldMode()

	// Let the user know that we're ready, then wait for the "GO" signal.
	m.hw.PlaySound("/sounds/ready.wav")
	m.startWG.Wait()

	startTime := time.Now()

	// Store initial heading.  Images 0, 1, 2, 3 will be at +45,
	// +135, +225 and +315 w.r.t. this initial heading.
	initialHeading := m.hw.CurrentHeading()
	cornerHeadings := [4]float64{
		initialHeading + 45,
		initialHeading + 45 + 90,
		initialHeading + 45 + 90 + 90,
		initialHeading + 45 + 90 + 90 + 90,
	}

	// If we don't already know the visit order, i.e. this is our
	// first run.
	if m.visitOrder == nil {

		var (
			hsv [4]gocv.Mat
			err error
		)

		// Turn to take photos of the four corners.
		for ii, cornerHeading := range cornerHeadings {
			hh.SetHeading(cornerHeading)
			residualError, _ := hh.Wait(ctx)
			fmt.Println("NEBULA: Completed turn, residual error: ", residualError)
			hsv[ii], err = m.takePicture()
			if err != nil {
				m.fatal(err)
			}
			defer func(ii int) {
				_ = hsv[ii].Close()
			}(ii)
		}
		// Calculate the order we need to visit the corners, by
		// matching photos to colours.
		m.visitOrder = m.calculateVisitOrder(hsv)
	}

	for ii, index := range m.visitOrder {

		fmt.Println("NEBULA: Next target ball: ", m.config.Sequence[ii])
		m.announceTargetBall(ii)

		// Rotating phase.
		hh.SetThrottle(0)
		hh.SetHeading(cornerHeadings[index])
		residualError, _ := hh.Wait(ctx)

		fmt.Println("NEBULA: Completed turn, residual error: ", residualError)

		time.Sleep(100 * time.Millisecond)

		// Advancing phase.
		advanceStartL, advanceStartR := m.hw.CurrentMotorDistances()

		distanceTraveledMM := func() float64 {
			l, r := m.hw.CurrentMotorDistances()
			return (l - advanceStartL + r - advanceStartR) / 2
		}

		hh.SetThrottle(m.config.ForwardSpeed)

		movingFast := true
		for ctx.Err() == nil {
			readSensors()
			traveledMM := distanceTraveledMM()
			fmt.Println("NEBULA: Target colour:", m.config.Sequence[ii], "Advancing", traveledMM, "mm")
			closeEnoughToSlowDown := traveledMM > 300

			closeEnough := closeEnoughToSlowDown &&
				(frontLeft.BestGuess() <= m.config.ForwardCornerDetectionThreshold) &&
				(frontRight.BestGuess() <= m.config.ForwardCornerDetectionThreshold)

			if closeEnough {
				fmt.Println("NEBULA: Reached target colour:", m.config.Sequence[ii], "in", time.Since(startTime))
				break
			}

			// We're approaching the ball but not yet close enough.
			if movingFast && closeEnoughToSlowDown {
				fmt.Println("NEBULA: Slowing...")
				hh.SetThrottle(m.config.ForwardSlowSpeed)
				movingFast = false
			}
		}

		if ii == 3 {
			// We've finished.
			time.Sleep(50 * time.Millisecond) // Hack: we seem to stop a little early on the last target.
			hh.SetThrottle(0)
			break
		}

		// Reversing phase.
		hh.SetThrottle(-m.config.ForwardSpeed)
		reverseStart := time.Now()
		for ctx.Err() == nil {
			readSensors()

			distanceFromMiddle := distanceTraveledMM()
			fmt.Printf("NEBULA: Reversing for %.2fs, distance from middle: %.0fmm\n",
				time.Since(reverseStart).Seconds(), distanceFromMiddle)
			if distanceFromMiddle < 100 {
				hh.SetThrottle(-m.config.ForwardSlowSpeed)
			}
			if distanceFromMiddle < 10 {
				break
			}
		}
	}
}

func (m *NebulaMode) announceTargetBall(ii int) {
	m.hw.PlaySound(
		fmt.Sprintf("/sounds/%vball.wav", m.config.Sequence[ii]),
	)
}

func (m *NebulaMode) stopSequence() {
	if !m.running {
		fmt.Println("NEBULA: Not running")
		return
	}
	fmt.Println("NEBULA: Stopping sequence...")

	m.cancelSequence()
	m.cancelSequence = nil
	m.sequenceWG.Wait()
	m.running = false
	atomic.StoreInt32(&m.paused, 0)

	m.hw.StopMotorControl()

	fmt.Println("NEBULA: Stopped sequence...")
}

func (m *NebulaMode) pauseOrResumeSequence() {
	if atomic.LoadInt32(&m.paused) == 1 {
		fmt.Println("NEBULA: Resuming sequence...")
		atomic.StoreInt32(&m.paused, 0)
	} else {
		fmt.Println("NEBULA: Pausing sequence...")
		atomic.StoreInt32(&m.paused, 1)
	}
}

func (m *NebulaMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}
