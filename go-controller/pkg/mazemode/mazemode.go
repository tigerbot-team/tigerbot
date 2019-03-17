package mazemode

import (
	"context"
	"math"
	"sync"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"

	"fmt"
	"log"
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
		fmt.Println("Tunable:", t.Name, "=", t.Value)
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
						fmt.Println("Getting ready!")
						m.startWG.Add(1)
						m.startSequence()
					case joystick.ButtonSquare:
						m.stopSequence()
						fmt.Println("Run time:", time.Since(startTime))
					case joystick.ButtonTriangle:
						m.pauseOrResumeSequence()
					}
				} else {
					switch event.Number {
					case joystick.ButtonR1:
						fmt.Println("GO!")
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
		fmt.Println("Already running")
		return
	}

	fmt.Println("Starting sequence...")
	m.running = true
	atomic.StoreInt32(&m.paused, 0)

	seqCtx, cancel := context.WithCancel(context.Background())
	m.cancelSequence = cancel
	m.sequenceWG.Add(1)
	go m.runSequence(seqCtx)
}

func (m *MazeMode) runSequence(ctx context.Context) {
	defer m.sequenceWG.Done()
	defer fmt.Println("Exiting sequence loop")

	// Create time-of-flight reading filters; should filter out any stray readings.
	var filters []*Filter
	for i := 0; i < 6; i++ {
		filters = append(filters, &Filter{})
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

			forwardGuess := sum / 2
			fwdThresh := m.turnEntryThreshMM.Get()
			if numGoodForwardReadings > 0 {
				fmt.Printf("MAZE: Wall at %dmm vs thresh %dmm\n", forwardGuess, fwdThresh)
			}
			if numGoodForwardReadings > 0 && forwardGuess < fwdThresh {
				log.Println("Reached wall in front")
				break
			}

			// Ramp up the speed on the straights...
			if forwardGuess > m.frontDistanceSpeedUpThreshMM.Get() {
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

			// TODO Stay in middle of walls

			//// Calculate our translational error.  We do our best to deal with missing sensor readings.
			//if leftRear.IsGood() && rightFore.IsGood() {
			//	// We have readings from both sides of the bot, try to stay in the middle.
			//	leftGuess := float64(leftRear.BestGuess())
			//	if leftFore.IsGood() {
			//		leftGuess = math.Min(leftGuess, frontLeftHorizEstMMs)
			//	}
			//	rightGuess := float64(rightFore.BestGuess())
			//	if frontRight.IsGood() {
			//		rightGuess = math.Min(rightGuess, frontRightHorizEstMMs)
			//	}
			//
			//	// Since we know we're in the middle, update the target clearance with actual measured values.
			//	translationErrorMMs = float64(leftGuess - rightGuess)
			//
			//	actualClearance := float64(leftGuess+rightGuess) / 2
			//	targetSideClearance = targetSideClearance*0.95 + actualClearance*0.05
			//} else if leftRear.IsGood() {
			//	leftGuess := float64(leftRear.BestGuess())
			//	if leftFore.IsGood() {
			//		leftGuess = math.Min(leftGuess, frontLeftHorizEstMMs)
			//	}
			//	translationErrorMMs = leftGuess - targetSideClearance
			//
			//	// Since we're not sure where we are, slowly go back to the default clearance.
			//	targetSideClearance = (targetSideClearance*clearandReturnFactor + clearanceMMs*(100-clearandReturnFactor)) / 100
			//} else if rightFore.IsGood() {
			//	rightGuess := float64(rightFore.BestGuess())
			//	if frontRight.IsGood() {
			//		rightGuess = math.Min(rightGuess, frontRightHorizEstMMs)
			//	}
			//	translationErrorMMs = targetSideClearance - rightGuess
			//
			//	// Since we're not sure where we are, slowly go back to the default clearance.
			//	targetSideClearance = (targetSideClearance*clearandReturnFactor + clearanceMMs*(100-clearandReturnFactor)) / 100
			//} else {
			//	// No idea, dissipate the error so we don't.
			//	translationErrorMMs = translationErrorMMs * 0.8
			//}
			//
			//var leftRotErr, rightRotError float64
			//rotErrGood := false
			//if leftFore.IsGood() && leftRear.IsGood() {
			//	leftRotErr = frontLeftHorizEstMMs - float64(leftRear.BestGuess())
			//	rotErrGood = true
			//}
			//if frontRight.IsGood() && rightFore.IsGood() {
			//	rightRotError = frontRightHorizEstMMs - float64(rightFore.BestGuess())
			//	rotErrGood = true
			//}
			//if rotErrGood {
			//	// Prefer the smaller magnitude error to avoid problems where one of the walls falls away...
			//	if math.Abs(leftRotErr) < math.Abs(rightRotError) {
			//		rotationErrorMMs = leftRotErr*0.8 - rightRotError*0.2
			//	} else {
			//		rotationErrorMMs = -rightRotError*0.8 + leftRotErr*0.2
			//	}
			//} else {
			//	rotationErrorMMs *= 0.9
			//}
			//
			//// positive if we're too far right
			//txErrorSq := math.Copysign(translationErrorMMs*translationErrorMMs, translationErrorMMs)
			//// positive if we're rotated too far clockwise
			//rotErrorSq := math.Copysign(rotationErrorMMs*rotationErrorMMs, rotationErrorMMs)
			//
			//scaledTxErr := txErrorSq * speed / 20000
			//scaledRotErr := rotErrorSq * speed / 1000
			//
			//fmt.Printf("MAZE: Control: S %.1f R %.2f T %.2f\n", speed/127, scaledRotErr/127, scaledTxErr/127)
			//m.headingHolder.SetControlInputs(-scaledRotErr/127, speed/127, -scaledTxErr/127)
		}

		hh.SetThrottle(0)

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
		_ = hh.Wait(ctx)

		// Flush the filters.
		readSensors()
		readSensors()
		readSensors()
		readSensors()
		readSensors()

		rotationEstimates := []float64{}
		if leftFore.BestGuess() < 350 && leftRear.BestGuess() < 350 {
			rotationEstimates = append(rotationEstimates, float64(leftFore.BestGuess()-leftRear.BestGuess()))
		}
		if rightFore.BestGuess() < 350 && rightRear.BestGuess() < 350 {
			rotationEstimates = append(rotationEstimates, float64(-rightFore.BestGuess()+rightRear.BestGuess()))
		}
		if len(rotationEstimates) > 0 {
			var sum float64
			for _, r := range rotationEstimates {
				sum += r
			}
			avg := sum / float64(len(rotationEstimates))
			rotEst := math.Atan(avg/110) * 360 / (2 * math.Pi)
			fmt.Printf("MAZE: Estimated offset: %.2f degrees\n", rotEst)

			if rotEst > 1 {
				rotEst = 1
			} else if rotEst < -1 {
				rotEst = -1
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
