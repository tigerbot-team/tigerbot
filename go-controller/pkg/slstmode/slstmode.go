package slstmode

import (
	"context"
	"math"
	"sync"

	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
	. "github.com/tigerbot-team/tigerbot/go-controller/pkg/tunable"
)

type SLSTMode struct {
	Propeller      propeller.Interface
	cancel         context.CancelFunc
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

	running        bool
	cancelSequence context.CancelFunc
	sequenceDone   chan struct{}

	paused int32

	tunables Tunables

	cornerSensorOffset      *Tunable
	cornerSensorAngleOffset *Tunable
	clearanceReturnFactor   *Tunable

	frontDistanceStopThresh     *Tunable
	cornerDistanceSpeedUpThresh *Tunable
	baseSpeed                   *Tunable
	topSpeed                    *Tunable
	speedRampUp                 *Tunable
	speedRampDown               *Tunable
}

func New(propeller propeller.Interface) *SLSTMode {
	mm := &SLSTMode{
		Propeller:      propeller,
		joystickEvents: make(chan *joystick.Event),
	}

	mm.cornerSensorOffset = mm.tunables.Create("Corner sensor offset", -12)
	mm.cornerSensorAngleOffset = mm.tunables.Create("Corner sensor angle offset", -8)
	mm.clearanceReturnFactor = mm.tunables.Create("Clearance return factor", 90)

	mm.frontDistanceStopThresh = mm.tunables.Create("Front distance stop threshold", 200)
	mm.cornerDistanceSpeedUpThresh = mm.tunables.Create("Corner distance speed up thresh", 70)
	mm.baseSpeed = mm.tunables.Create("Base speed", 35)
	mm.topSpeed = mm.tunables.Create("Top speed", 127)
	mm.speedRampUp = mm.tunables.Create("Speed ramp up ", 10)
	mm.speedRampDown = mm.tunables.Create("Speed ramp down", 10)

	return mm
}

func (m *SLSTMode) Name() string {
	return "SLST mode"
}

func (m *SLSTMode) Start(ctx context.Context) {
	m.stopWG.Add(1)
	var loopCtx context.Context
	loopCtx, m.cancel = context.WithCancel(ctx)
	go m.loop(loopCtx)
}

func (m *SLSTMode) Stop() {
	m.cancel()
	m.stopWG.Wait()

	for _, t := range m.tunables.All {
		fmt.Println("Tunable:", t.Name, "=", t.Value)
	}
}

func (m *SLSTMode) loop(ctx context.Context) {
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
						startTime = time.Now()
						m.startSequence()
					case joystick.ButtonSquare:
						m.stopSequence()
						fmt.Println("Run time:", time.Since(startTime))
					case joystick.ButtonTriangle:
						m.pauseOrResumeSequence()
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

func (m *SLSTMode) startSequence() {
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

func (m *SLSTMode) runSequence(ctx context.Context) {
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

	defer fmt.Println("Exiting sequence loop")

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
			if err == tofsensor.ErrMeasurementInvalid {
				reading = "<invalid>"
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
	_ = forwardRight
	_ = forwardLeft
	_ = sideLeft
	_ = sideRight

	readSensors()
	readSensors()
	readSensors()
	readSensors()
	readSensors()

	const (
		wallSeparationMMs = 560
		botWidthMMs       = 200
		clearanceMMs      = (wallSeparationMMs - botWidthMMs) / 2
	)

	var targetSideClearance float64 = clearanceMMs
	var translationErrorMMs float64 // positive if we're too far right
	var rotationErrorMMs float64    // positive if we're rotated too far clockwise

	var speed float64 = float64(m.baseSpeed.Get())

	for ctx.Err() == nil {
		// Main loop, follow the walls.

		m.sleepIfPaused(ctx)
		baseSpeed := float64(m.baseSpeed.Get())
		readSensors()
		//
		//// If we reach a wall in front, stop!
		//if !forward.IsFar() {
		//	forwardGuess := forward.BestGuess()
		//	if forwardGuess < m.frontDistanceStopThresh.Get() &&
		//		// In case of spurious reading from front sensor, check the others too
		//		(forwardLeft.BestGuess() < m.frontDistanceStopThresh.Get() ||
		//			forwardRight.BestGuess() < m.frontDistanceStopThresh.Get()) {
		//		log.Println("Reached wall in front")
		//		break
		//	}
		//}

		cornerSensorOffset := m.cornerSensorOffset.Get()
		cornerSensorAngleOffset := float64(m.cornerSensorAngleOffset.Get())
		frontLeftHorizEstMMs := float64(forwardLeft.BestGuess()+cornerSensorOffset) *
			(100.0 + cornerSensorAngleOffset) / 100 / math.Sqrt2
		frontRightHorizEstMMs := float64(forwardRight.BestGuess()+cornerSensorOffset) *
			(100.0 + cornerSensorAngleOffset) / 100 / math.Sqrt2
		clearandReturnFactor := float64(m.clearanceReturnFactor.Get())

		// Calculate our translational error.  We do our best to deal with missing sensor readings.
		if sideLeft.IsGood() && sideRight.IsGood() {
			// We have readings from both sides of the bot, try to stay in the middle.
			leftGuess := float64(sideLeft.BestGuess())
			if forwardLeft.IsGood() {
				leftGuess = math.Min(leftGuess, frontLeftHorizEstMMs)
			}
			rightGuess := float64(sideRight.BestGuess())
			if forwardRight.IsGood() {
				rightGuess = math.Min(rightGuess, frontRightHorizEstMMs)
			}

			// Since we know we're in the middle, update the target clearance with actual measured values.
			translationErrorMMs = float64(leftGuess - rightGuess)

			actualClearance := float64(leftGuess+rightGuess) / 2
			targetSideClearance = targetSideClearance*0.95 + actualClearance*0.05
		} else if sideLeft.IsGood() {
			leftGuess := float64(sideLeft.BestGuess())
			if forwardLeft.IsGood() {
				leftGuess = math.Min(leftGuess, frontLeftHorizEstMMs)
			}
			translationErrorMMs = leftGuess - targetSideClearance

			// Since we're not sure where we are, slowly go back to the default clearance.
			targetSideClearance = (targetSideClearance*clearandReturnFactor + clearanceMMs*(100-clearandReturnFactor)) / 100
		} else if sideRight.IsGood() {
			rightGuess := float64(sideRight.BestGuess())
			if forwardRight.IsGood() {
				rightGuess = math.Min(rightGuess, frontRightHorizEstMMs)
			}
			translationErrorMMs = targetSideClearance - rightGuess

			// Since we're not sure where we are, slowly go back to the default clearance.
			targetSideClearance = (targetSideClearance*clearandReturnFactor + clearanceMMs*(100-clearandReturnFactor)) / 100
		} else {
			// No idea, dissipate the error so we don't.
			translationErrorMMs = translationErrorMMs * 0.8
		}

		var leftRotErr, rightRotError float64
		rotErrGood := false
		if forwardLeft.IsGood() && sideLeft.IsGood() {
			leftRotErr = frontLeftHorizEstMMs - float64(sideLeft.BestGuess())
			rotErrGood = true
		}
		if forwardRight.IsGood() && sideRight.IsGood() {
			rightRotError = frontRightHorizEstMMs - float64(sideRight.BestGuess())
			rotErrGood = true
		}
		if rotErrGood {
			// Prefer the smaller magnitude error to avoid problems where one of the walls falls away...
			if math.Abs(leftRotErr) < math.Abs(rightRotError) {
				rotationErrorMMs = leftRotErr*0.8 - rightRotError*0.2
			} else {
				rotationErrorMMs = -rightRotError*0.8 + leftRotErr*0.2
			}
		} else {
			rotationErrorMMs *= 0.9
		}
		fmt.Printf("Translation error %.1f Rotation error %.1f\n", translationErrorMMs, rotationErrorMMs)

		// positive if we're too far right
		txErrorSq := math.Copysign(translationErrorMMs*translationErrorMMs, translationErrorMMs)
		// positive if we're rotated too far clockwise
		rotErrorSq := math.Copysign(rotationErrorMMs*rotationErrorMMs, rotationErrorMMs)

		if -40 < translationErrorMMs && translationErrorMMs < 40 &&
			-20 < rotationErrorMMs && rotationErrorMMs < 20 {
			if speed < float64(m.topSpeed.Get()) {
				speed += float64(m.speedRampUp.Get())
			}
		} else if -80 > translationErrorMMs || translationErrorMMs > 80 ||
			-40 > rotationErrorMMs || rotationErrorMMs > 40 {
			if speed > baseSpeed {
				speed -= float64(m.speedRampDown.Get())
			}
		}

		scaledTxErr := txErrorSq * speed / 10000
		scaledRotErr := rotErrorSq * speed / 10000

		fl := clamp(speed-scaledTxErr-scaledRotErr, speed*2)
		fr := clamp(speed+scaledTxErr+scaledRotErr, speed*2)
		bl := clamp(speed+scaledTxErr-scaledRotErr, speed*2)
		br := clamp(speed-scaledTxErr+scaledRotErr, speed*2)

		fmt.Printf("Speeds: FL=%d FR=%d BL=%d BR=%d\n", fl, fr, bl, br)
		m.Propeller.SetMotorSpeeds(fl, fr, bl, br)
	}

	fmt.Println("Stopping...")
	m.Propeller.SetMotorSpeeds(0, 0, 0, 0)
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
	if len(f.samples) > 5 {
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
	return f.BestGuess() > 0 && f.BestGuess() < 450
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
	return goodSamples[len(goodSamples)/2]
}

func (m *SLSTMode) sleepIfPaused(ctx context.Context) {
	for atomic.LoadInt32(&m.paused) == 1 && ctx.Err() == nil {
		m.Propeller.SetMotorSpeeds(0, 0, 0, 0)
		time.Sleep(100 * time.Millisecond)
	}
}

func (m *SLSTMode) stopSequence() {
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

func (m *SLSTMode) pauseOrResumeSequence() {
	if atomic.LoadInt32(&m.paused) == 1 {
		fmt.Println("Resuming sequence...")
		atomic.StoreInt32(&m.paused, 0)
	} else {
		fmt.Println("Pausing sequence...")
		atomic.StoreInt32(&m.paused, 1)
	}
}

func (m *SLSTMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}
