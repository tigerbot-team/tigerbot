package slstmode

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

type SLSTMode struct {
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

func New(hw hardware.Interface) *SLSTMode {
	mm := &SLSTMode{
		hw:             hw,
		joystickEvents: make(chan *joystick.Event),
	}

	mm.turnEntryThreshMM = mm.tunables.Create("Turn entry threshold", 20)
	mm.turnThrottlePct = mm.tunables.Create("Turn throttle %", 0)

	mm.frontDistanceSpeedUpThreshMM = mm.tunables.Create("Front distance speed up thresh", 350)
	mm.baseSpeedPct = mm.tunables.Create("Base speed %", 10)
	mm.topSpeedPct = mm.tunables.Create("Top speed %", 15)
	mm.speedRampUp = mm.tunables.Create("Speed ramp up %/loop", 1)
	mm.speedRampDown = mm.tunables.Create("Speed ramp down %/loop", 1)

	return mm
}

func (s *SLSTMode) Name() string {
	return "SPEED MODE"
}

func (s *SLSTMode) StartupSound() string {
	return "/sounds/slstmode.wav"
}

func (s *SLSTMode) Start(ctx context.Context) {
	s.stopWG.Add(1)
	var loopCtx context.Context
	loopCtx, s.cancel = context.WithCancel(ctx)
	go s.loop(loopCtx)
}

func (s *SLSTMode) Stop() {
	s.cancel()
	s.stopWG.Wait()

	for _, t := range s.tunables.All {
		fmt.Println("Tunable:", t.Name, "=", t.Value)
	}
}

func (s *SLSTMode) loop(ctx context.Context) {
	defer s.stopWG.Done()

	var startTime time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-s.joystickEvents:
			switch event.Type {
			case joystick.EventTypeButton:
				if event.Value == 1 {
					switch event.Number {
					case joystick.ButtonR1:
						fmt.Println("Getting ready!")
						s.startWG.Add(1)
						s.startSequence()
					case joystick.ButtonSquare:
						s.stopSequence()
						fmt.Println("Run time:", time.Since(startTime))
					case joystick.ButtonTriangle:
						s.pauseOrResumeSequence()
					}
				} else {
					switch event.Number {
					case joystick.ButtonR1:
						fmt.Println("GO!")
						startTime = time.Now()
						s.startWG.Done()
					}
				}
			case joystick.EventTypeAxis:
				switch event.Number {
				case joystick.AxisDPadX:
					if event.Value > 0 {
						// Right
						s.tunables.SelectNext()
					} else if event.Value < 0 {
						// Left
						s.tunables.SelectPrev()
					}
				case joystick.AxisDPadY:
					if event.Value < 0 {
						// Up
						s.tunables.Current().Add(1)
					} else if event.Value > 0 {
						// Down
						s.tunables.Current().Add(-1)
					}
				}
			}
		}

	}
}

func (s *SLSTMode) startSequence() {
	if s.running {
		fmt.Println("Already running")
		return
	}

	fmt.Println("Starting sequence...")
	s.running = true
	atomic.StoreInt32(&s.paused, 0)

	seqCtx, cancel := context.WithCancel(context.Background())
	s.cancelSequence = cancel
	s.sequenceWG.Add(1)
	go s.runSequence(seqCtx)
}

func (s *SLSTMode) runSequence(ctx context.Context) {
	defer s.sequenceWG.Done()
	defer fmt.Println("Exiting sequence loop")

	// Create time-of-flight reading filters; should filter out any stray readings.
	var filters []*Filter
	for i := 0; i < 6; i++ {
		filters = append(filters, &Filter{})
	}

	readSensors := func() {
		// Read the sensors
		msg := ""
		readings := s.hw.CurrentDistanceReadings()
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
			msg += fmt.Sprintf("%s:=%5s/%5dmm ", filters[j].Name, prettyPrinted, filters[j].BestGuess())
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

	// We use the absolute heading hold mode so we can do things like "turn right 45 degrees".
	hh := s.hw.StartHeadingHoldMode()

	// Let the user know that we're ready, then wait for the "GO" signal.
	s.hw.PlaySound("/sounds/ready.wav")
	s.startWG.Wait()

	const (
		wallSeparationMMs = 550
		botWidthMMs       = 200
		clearanceMMs      = (wallSeparationMMs - botWidthMMs) / 2
	)

	//var targetSideClearance float64 = clearanceMMs
	//var translationErrorMMs float64 // positive if we're too far right
	//var rotationErrorMMs float64    // positive if we're rotated too far clockwise

	lastTurnSign := 0

	for ctx.Err() == nil {
		// Main loop, alternates between following the walls until we detect a wall in front, and then
		// making turns.

		var speed float64 = float64(s.baseSpeedPct.Get())

		fmt.Println("Following the walls...")
		for ctx.Err() == nil {
			for atomic.LoadInt32(&s.paused) == 1 && ctx.Err() == nil {
				// Bot is paused.
				hh.SetThrottle(0)
				time.Sleep(100 * time.Millisecond)
			}

			baseSpeed := float64(s.baseSpeedPct.Get())
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
			if numGoodForwardReadings > 0 && forwardGuess < s.turnEntryThreshMM.Get() {
				log.Println("Reached wall in front")
				break
			}

			// Ramp up the speed on the straights...
			if forwardGuess > s.frontDistanceSpeedUpThreshMM.Get() {
				speed += float64(s.speedRampUp.Get())
			} else {
				speed -= float64(s.speedRampDown.Get())
			}
			if speed < baseSpeed {
				speed = baseSpeed
			}
			if speed > float64(s.topSpeedPct.Get()) {
				speed = float64(s.topSpeedPct.Get())
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
			//fmt.Printf("Control: S %.1f R %.2f T %.2f\n", speed/127, scaledRotErr/127, scaledTxErr/127)
			//s.headingHolder.SetControlInputs(-scaledRotErr/127, speed/127, -scaledTxErr/127)
		}

		hh.SetThrottle(0)

		fl := frontLeft.BestGuess()
		if fl > 300 {
			fl = 300
		}
		fr := frontRight.BestGuess()
		if fr > 300 {
			fr = 300
		}

		leftTurnConfidence := -1000*lastTurnSign + leftFore.BestGuess() + leftRear.BestGuess() + frontLeft.BestGuess() - frontRight.BestGuess()
		rightTurnConfidence := 1000*lastTurnSign + rightFore.BestGuess() + rightRear.BestGuess() - frontLeft.BestGuess() + frontRight.BestGuess()

		fmt.Println("Left confidence:", leftTurnConfidence, "Right confidence:", rightTurnConfidence)

		var sign float64
		if leftTurnConfidence > rightTurnConfidence {
			fmt.Println("Turning left...")
			sign = -1
		} else {
			fmt.Println("Turning right...")
			sign = 1
		}

		startHeading := s.hw.CurrentHeading()
		fmt.Println("Turn start heading:", startHeading)
		hh.AddHeadingDelta(sign * 45)
		_ = hh.Wait(ctx)

		lastTurnSign = int(sign)
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

func (s *SLSTMode) stopSequence() {
	if !s.running {
		fmt.Println("Not running")
		return
	}
	fmt.Println("Stopping sequence...")

	s.cancelSequence()
	s.cancelSequence = nil
	s.sequenceWG.Wait()
	s.running = false
	atomic.StoreInt32(&s.paused, 0)

	s.hw.StopMotorControl()

	fmt.Println("Stopped sequence...")
}

func (s *SLSTMode) pauseOrResumeSequence() {
	if atomic.LoadInt32(&s.paused) == 1 {
		fmt.Println("Resuming sequence...")
		atomic.StoreInt32(&s.paused, 0)
	} else {
		fmt.Println("Pausing sequence...")
		atomic.StoreInt32(&s.paused, 1)
	}
}

func (s *SLSTMode) OnJoystickEvent(event *joystick.Event) {
	s.joystickEvents <- event
}
