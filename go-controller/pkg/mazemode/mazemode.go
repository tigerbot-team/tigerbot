package mazemode

import (
	"context"
	"math"
	"sync"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"

	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
	. "github.com/tigerbot-team/tigerbot/go-controller/pkg/tunable"
)

type MazeMode struct {
	hw hardware.Interface

	cancel         context.CancelFunc
	startWG        sync.WaitGroup
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

	running        bool
	cancelSequence context.CancelFunc
	sequenceWG     sync.WaitGroup

	paused int32

	tunables Tunables

	turnEntryThreshMM   *Tunable
	turnExitThresh      *Tunable
	turnExitRatioThresh *Tunable

	turnThrottlePct *Tunable

	frontDistanceSpeedUpThreshMM *Tunable
	baseSpeedPct                 *Tunable
	topSpeedPct                  *Tunable
	speedRampUp                  *Tunable
	speedRampDown                *Tunable
}

func New(hw hardware.Interface) *MazeMode {
	mm := &MazeMode{
		hw:             hw,
		joystickEvents: make(chan *joystick.Event),
	}

	mm.turnEntryThreshMM = mm.tunables.Create("Turn entry threshold", 280)
	mm.turnThrottlePct = mm.tunables.Create("Turn throttle %", 0)

	mm.frontDistanceSpeedUpThreshMM = mm.tunables.Create("Front distance speed up thresh", 350)
	mm.baseSpeedPct = mm.tunables.Create("Base speed %", 10)
	mm.topSpeedPct = mm.tunables.Create("Top speed %", 15)
	mm.speedRampUp = mm.tunables.Create("Speed ramp up %/loop", 1)
	mm.speedRampDown = mm.tunables.Create("Speed ramp down %/loop", 1)

	return mm
}

func (m *MazeMode) Name() string {
	return "MAZE MODE"
}

func (m *MazeMode) StartupSound() string {
	return "/sounds/mazemode.wav"
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

	for _, t := range m.tunables.All {
		fmt.Println("MAZE: Tunable:", t.Name, "=", t.Value)
	}
}

func (m *MazeMode) loop(ctx context.Context) {
	defer m.stopWG.Done()

	var startTime time.Time

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
						fmt.Println("MAZE: Getting ready!")
						m.startWG.Add(1)
						m.startSequence()
					case joystick.ButtonSquare:
						m.stopSequence()
						fmt.Println("MAZE: Run time:", time.Since(startTime))
					case joystick.ButtonTriangle:
						m.pauseOrResumeSequence()
					}
				} else {
					switch event.Number {
					case joystick.ButtonR1:
						fmt.Println("MAZE: GO!")
						startTime = time.Now()
						m.startWG.Done()
					}
				}
			case joystick.EventTypeAxis:
				switch event.Number {
				case joystick.AxisDPadX:
					if event.Value > 0 {
						// Right
						m.tunables.SelectNext()
					} else if event.Value < 0 {
						// Left
						m.tunables.SelectPrev()
					}
				case joystick.AxisDPadY:
					if event.Value < 0 {
						// Up
						m.tunables.Current().Add(1)
					} else if event.Value > 0 {
						// Down
						m.tunables.Current().Add(-1)
					}
				}
			}
		}

	}
}

func (m *MazeMode) startSequence() {
	if m.running {
		fmt.Println("MAZE: Already running")
		return
	}

	fmt.Println("MAZE: Starting sequence...")
	m.running = true
	atomic.StoreInt32(&m.paused, 0)

	seqCtx, cancel := context.WithCancel(context.Background())
	m.cancelSequence = cancel
	m.sequenceWG.Add(1)
	go m.runSequence(seqCtx)
}

func (m *MazeMode) runSequence(ctx context.Context) {
	defer m.sequenceWG.Done()
	defer fmt.Println("MAZE: Exiting sequence loop")

	// Create time-of-flight reading filters; should filter out any stray readings.
	var filters []*Filter
	for i := 0; i < 6; i++ {
		filters = append(filters, &Filter{})
	}

	var readings hardware.DistanceReadings

	readSensors := func() {
		// Read the sensors
		msg := "MAZE readings "
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

	flushSensors := func() {
		for _, f := range filters {
			f.Flush()
		}

		readSensors()
		readSensors()
		readSensors()
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

	// We use the absolute haeding hold mode so we can do things like "turn right 90 degrees".
	hh := m.hw.StartHeadingHoldMode()

	// Let the user know that we're ready, then wait for the "GO" signal.
	m.hw.PlaySound("/sounds/ready.wav")
	m.startWG.Wait()

	const (
		wallSeparationMMs = 550
		botWidthMMs       = 200
		clearanceMMs      = (wallSeparationMMs - botWidthMMs) / 2
	)

	//var targetSideClearance float64 = clearanceMMs
	//var translationErrorMMs float64 // positive if we're too far right
	//var rotationErrorMMs float64    // positive if we're rotated too far clockwise

	for ctx.Err() == nil {
		// Main loop, alternates between following the walls until we detect a wall in front, and then
		// making turns.

		var speed float64 = float64(m.baseSpeedPct.Get())

		fmt.Println("MAZE: Following the walls...")
		lastCorrectionTime := time.Now()
		for ctx.Err() == nil {
			for atomic.LoadInt32(&m.paused) == 1 && ctx.Err() == nil {
				// Bot is paused.
				hh.SetThrottle(0)
				time.Sleep(100 * time.Millisecond)
			}

			baseSpeed := float64(m.baseSpeedPct.Get())
			readSensors()

			// If we reach a wall in front, break out and do the turn.
			var numGoodForwardReadings int
			var sum int
			if frontLeft.IsGood() {
				numGoodForwardReadings++
				sum += frontLeft.BestGuess()
			}
			if frontRight.IsGood() {
				numGoodForwardReadings++
				sum += frontRight.BestGuess()
			}
			if frontLeft.IsGood() && frontRight.IsGood() {
				if math.Abs(float64(frontLeft.BestGuess()-frontRight.BestGuess())) > 150 {
					fmt.Println("MAZE: Large difference between front readings, false reading?")
					numGoodForwardReadings = 0
				}
			}

			var forwardGuess float64 = 2001
			if numGoodForwardReadings > 0 {
				fwdThresh := float64(m.turnEntryThreshMM.Get())
				fmt.Printf("MAZE: Wall at %.1fmm vs thresh %.1fmm\n", forwardGuess, fwdThresh)
				forwardGuess = float64(sum) / float64(numGoodForwardReadings)

				if numGoodForwardReadings > 0 && forwardGuess < fwdThresh {
					fmt.Println("MAZE: Reached wall in front")
					break
				}
			}

			if time.Since(lastCorrectionTime) > 500*time.Millisecond {
				num := 0
				sum := 0.0
				lfMMPerS := leftFore.MMPerSecond()
				if lfMMPerS != 0 {
					num += 1
					sum += lfMMPerS
				}
				rfMMPerS := rightFore.MMPerSecond()
				if rfMMPerS != 0 {
					num += 1
					sum -= rfMMPerS
				}
				lrMMPerS := leftRear.MMPerSecond()
				if lrMMPerS != 0 {
					num += 1
					sum += lrMMPerS
				}
				rrMMPerS := rightRear.MMPerSecond()
				if rrMMPerS != 0 {
					num += 1
					sum -= rrMMPerS
				}
				fmt.Printf("MAZE: MM/s estimates L: %.1f %.1f R: %.1f %.1f\n", lfMMPerS, lrMMPerS, rfMMPerS, rrMMPerS)
				if num > 0 {
					avg := sum / float64(num)
					correction := 0.01 * -avg * speed
					fmt.Printf("MAZE: Making correction: %.2f\n", correction)
					hh.AddHeadingDelta(correction)
					lastCorrectionTime = time.Now()
				}
			}

			// Ramp up the speed on the straights...
			if forwardGuess > float64(m.frontDistanceSpeedUpThreshMM.Get()) {
				speed += float64(m.speedRampUp.Get())
			} else {
				speed -= float64(m.speedRampDown.Get())
			}
			if speed < baseSpeed {
				speed = baseSpeed
			}
			if speed > float64(m.topSpeedPct.Get()) {
				speed = float64(m.topSpeedPct.Get())
			}

			hh.SetThrottle(speed / 100)
		}

		hh.SetThrottle(0)
		var rotationEstimates []float64

		flushSensors()

		frontRot := float64(frontLeft.BestGuess() - frontRight.BestGuess())
		if frontLeft.BestGuess() < 350 && frontRight.BestGuess() < 350 && math.Abs(frontRot) < 50 {
			fmt.Println("MAZE: Front pre-turn rotation estimate: ", frontRot)
			rotationEstimates = append(rotationEstimates, float64(frontRight.BestGuess()-frontLeft.BestGuess()))
		}

		leftTurnConfidence := leftFore.BestGuess() + leftRear.BestGuess()
		rightTurnConfidence := rightFore.BestGuess() + rightRear.BestGuess()

		fmt.Println("MAZE: Left confidence:", leftTurnConfidence, "Right confidence:", rightTurnConfidence)

		var sign float64
		if leftTurnConfidence > rightTurnConfidence {
			fmt.Println("MAZE: Turning left...")
			sign = -1
		} else {
			fmt.Println("MAZE: Turning right...")
			sign = 1
		}

		startHeading := m.hw.CurrentHeading()
		fmt.Println("MAZE: Turn start heading:", startHeading)

		hh.AddHeadingDelta(sign * 90)
		measuredErr, err := hh.Wait(ctx)
		if err != nil {
			fmt.Println("MAZE: Error from wait: ", err)
			return
		}

		flushSensors()

		if leftFore.BestGuess() < 100 {
			fmt.Println("MAZE: Too close to left wall, applying a delta")
			hh.AddHeadingDelta(.2)
		}
		if rightFore.BestGuess() < 100 {
			fmt.Println("MAZE: Too close to right wall, applying a delta")
			hh.AddHeadingDelta(-.2)
		}
		leftRot := float64(leftFore.BestGuess() - leftRear.BestGuess())
		if leftFore.BestGuess() < 350 && leftRear.BestGuess() < 350 && math.Abs(leftRot) < 50 {
			fmt.Println("MAZE: Left rotation estimate: ", leftRot)
			rotationEstimates = append(rotationEstimates, leftRot)
		}
		rightRot := float64(rightRear.BestGuess() - rightFore.BestGuess())
		if rightFore.BestGuess() < 350 && rightRear.BestGuess() < 350 && math.Abs(rightRot) < 50 {
			fmt.Println("MAZE: Right rotation estimate: ", leftRot)
			rotationEstimates = append(rotationEstimates, rightRot)
		}
		if len(rotationEstimates) > 0 {
			var sum float64
			for _, r := range rotationEstimates {
				sum += r
			}
			avg := sum / float64(len(rotationEstimates))
			rotEst := math.Atan(avg/110)*360/(2*math.Pi) - measuredErr
			fmt.Printf("MAZE: Estimated offset: %.2f degrees (mesaured %.2f)\n", rotEst, measuredErr)

			if rotEst > 1.5 {
				rotEst = 1.5
			} else if rotEst < -1.5 {
				rotEst = -1.5
			}
			hh.AddHeadingDelta(-rotEst)
		}
	}
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

type filterSample struct {
	mm   int
	time time.Time
}

const (
	bufSize       = 20
	recencyThresh = time.Millisecond * 90
)

type Filter struct {
	Name    string
	samples []filterSample
}

func (f *Filter) Accumulate(sample int, t time.Time) {
	f.samples = append(f.samples, filterSample{sample, t})
	if len(f.samples) > bufSize {
		f.samples = f.samples[1:]
	}
}

func (f *Filter) Flush() {
	f.samples = f.samples[:0]
}

func (f *Filter) recentSamples() []filterSample {
	var result []filterSample
	for _, s := range f.samples {
		if time.Since(s.time) < recencyThresh {
			result = append(result, s)
		}
	}
	return result
}

func (f *Filter) IsFar() bool {
	// Look backwards in the samples
	samples := f.recentSamples()
	for i := len(samples) - 1; i >= 0; i-- {
		if samples[i].mm > 400 {
			// 400mm is far by definition.
			return true
		}
		if samples[i].mm > 0 {
			// any recent non-far sample means we're not far.
			return false
		}
	}
	return true
}

func (f *Filter) IsGood() bool {
	return f.BestGuess() > 10 && f.BestGuess() < 1500
}

func (f *Filter) BestGuess() int {
	var goodSamples []int
	for _, s := range f.recentSamples() {
		if s.mm != 0 && s.mm < tofsensor.RangeTooFar {
			goodSamples = append(goodSamples, s.mm)
		}
	}
	if len(goodSamples) == 0 {
		return 0
	}
	sort.Ints(goodSamples)
	return goodSamples[len(goodSamples)/2]
}

func (f *Filter) MMPerSecond() float64 {
	var goodSamples2 []filterSample
	{
		var goodSamples []filterSample
		var min = tofsensor.RangeTooFar
		for _, s := range f.samples {
			if s.mm != 0 && s.mm < 400 {
				goodSamples = append(goodSamples, s)
				if s.mm < min {
					min = s.mm
				}
			}
		}
		for _, s := range goodSamples {
			if s.mm < min+10 {
				goodSamples2 = append(goodSamples2, s)
			}
		}
		if len(goodSamples2) < 10 {
			return 0
		}
	}
	last := goodSamples2[len(goodSamples2)-1]
	first := goodSamples2[0]
	dTime := last.time.Sub(first.time).Seconds()
	dMM := last.mm - first.mm
	if dTime == 0 {
		return 0
	}
	return float64(dMM) / dTime
}

func (m *MazeMode) stopSequence() {
	if !m.running {
		fmt.Println("MAZE: Not running")
		return
	}
	fmt.Println("MAZE: Stopping sequence...")

	m.cancelSequence()
	m.cancelSequence = nil
	m.sequenceWG.Wait()
	m.running = false
	atomic.StoreInt32(&m.paused, 0)

	m.hw.StopMotorControl()

	fmt.Println("MAZE: Stopped sequence...")
}

func (m *MazeMode) pauseOrResumeSequence() {
	if atomic.LoadInt32(&m.paused) == 1 {
		fmt.Println("MAZE: Resuming sequence...")
		atomic.StoreInt32(&m.paused, 0)
	} else {
		fmt.Println("MAZE: Pausing sequence...")
		atomic.StoreInt32(&m.paused, 1)
	}
}

func (m *MazeMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}
