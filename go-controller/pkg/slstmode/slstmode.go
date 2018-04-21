package slstmode

import (
	"context"
	"math"
	"sync"

	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
	. "github.com/tigerbot-team/tigerbot/go-controller/pkg/tunable"
)

type SLSTMode struct {
	i2cLock        sync.Mutex
	Propeller      propeller.Interface
	headingHolder  *headingholder.HeadingHolder
	cancel         context.CancelFunc
	startWG        sync.WaitGroup
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

	running        bool
	cancelSequence context.CancelFunc
	sequenceWG     sync.WaitGroup

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

	tnKp  *Tunable
	tnKi  *Tunable
	tnKd  *Tunable
	rotKp *Tunable
	rotKi *Tunable
	rotKd *Tunable
}

func New(propeller propeller.Interface) *SLSTMode {
	mm := &SLSTMode{
		Propeller:      propeller,
		joystickEvents: make(chan *joystick.Event),
	}

	mm.headingHolder = headingholder.New(&mm.i2cLock, propeller)

	mm.cornerSensorOffset = mm.tunables.Create("Corner sensor offset", -12)
	mm.cornerSensorAngleOffset = mm.tunables.Create("Corner sensor angle offset", -8)
	mm.clearanceReturnFactor = mm.tunables.Create("Clearance return factor", 90)

	mm.frontDistanceStopThresh = mm.tunables.Create("Front distance stop threshold", 200)
	mm.cornerDistanceSpeedUpThresh = mm.tunables.Create("Corner distance speed up thresh", 70)
	mm.baseSpeed = mm.tunables.Create("Base speed", 35)
	mm.topSpeed = mm.tunables.Create("Top speed", 127)
	mm.speedRampUp = mm.tunables.Create("Speed ramp up ", 3)
	mm.speedRampDown = mm.tunables.Create("Speed ramp down", 2)

	mm.tnKp = mm.tunables.Create("Translation Kp (thousandths)", 50)
	mm.tnKi = mm.tunables.Create("Translation Ki (ten-thousandths)", 10)
	mm.tnKd = mm.tunables.Create("Translation Kd (ten-thousandths)", -10)
	mm.rotKp = mm.tunables.Create("Rotation Kp (thousandths)", 90)
	mm.rotKi = mm.tunables.Create("Rotation Ki (ten-thousandths)", 0)
	mm.rotKd = mm.tunables.Create("Rotation Kd (ten-thousandths)", -20)

	return mm
}

func (m *SLSTMode) Name() string {
	return "SLST mode"
}

func (m *SLSTMode) StartupSound() string {
	return "/sounds/slstmode.wav"
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
						m.startSequence()
						m.startWG.Add(1)
					case joystick.ButtonSquare:
						m.stopSequence()
						fmt.Println("Run time:", time.Since(startTime))
					case joystick.ButtonTriangle:
						m.pauseOrResumeSequence()
					}
				} else {
					switch event.Number {
					case joystick.ButtonR1:
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
	m.sequenceWG.Add(1)
	go m.runSequence(seqCtx)
}

func (m *SLSTMode) runSequence(ctx context.Context) {
	defer m.sequenceWG.Done()

	m.i2cLock.Lock()
	mx, err := mux.New("/dev/i2c-1")
	m.i2cLock.Unlock()
	if err != nil {
		fmt.Println("Failed to open mux", err)
		return
	}

	var tofs []tofsensor.Interface
	defer func() {
		m.i2cLock.Lock()
		for _, tof := range tofs {
			tof.Close()
		}
		m.i2cLock.Unlock()
	}()
	for _, port := range []int{
		mux.BusTOFSideLeft,
		mux.BusTOFFrontLeft,
		mux.BusTOFForward,
		mux.BusTOFFrontRight,
		mux.BusTOFSideRight,
	} {
		m.i2cLock.Lock()
		tof, err := tofsensor.NewMuxed("/dev/i2c-1", 0x29, mx, port)
		m.i2cLock.Unlock()
		if err != nil {
			fmt.Println("Failed to open sensor", err)
			return
		}
		m.i2cLock.Lock()
		err = tof.StartContinuousMeasurements()
		m.i2cLock.Unlock()
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
		start := time.Now()
		fmt.Print("ToF: ")
		for j, tof := range tofs {
			reading := "-"
			m.i2cLock.Lock()
			readingInMM, err := tof.GetNextContinuousMeasurement()
			m.i2cLock.Unlock()
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
		fmt.Println("in", time.Since(start))
	}

	sideLeft := filters[0]
	sideLeft.Name = "L"
	frontLeft := filters[1]
	frontLeft.Name = "FL"
	front := filters[2]
	front.Name = "F"
	frontRight := filters[3]
	frontRight.Name = "FR"
	sideRight := filters[4]
	sideRight.Name = "R"
	_ = frontRight
	_ = frontLeft
	_ = sideLeft
	_ = sideRight

	readSensors()
	readSensors()

	m.sequenceWG.Add(1)
	go m.headingHolder.Loop(ctx, &m.sequenceWG)

	m.startWG.Wait()

	const (
		wallSeparationMMs = 560
		botWidthMMs       = 200
		clearanceMMs      = (wallSeparationMMs - botWidthMMs) / 2
	)

	var lastTnError float64  // positive if we're too far right
	var lastRotError float64 // positive if we're rotated too far clockwise
	var tnErrorInt float64
	var rotErrorInt float64
	var lastReadingTime = time.Now()

	var speed float64 = float64(m.baseSpeed.Get())

	for ctx.Err() == nil {
		// Main loop, follow the walls.

		m.sleepIfPaused(ctx)
		baseSpeed := float64(m.baseSpeed.Get())

		var readingTime = time.Now()
		readSensors()

		// FIXME: spurious triggers if there's a speck of something reflective on the floor!
		//// If we reach a wall in front, stop!
		//if !front.IsFar() {
		//	frontGuess := front.BestGuess()
		//	if frontGuess < m.frontDistanceStopThresh.Get() &&
		//		// In case of spurious reading from front sensor, check the others too
		//		(frontLeft.BestGuess() < m.frontDistanceStopThresh.Get() ||
		//			frontRight.BestGuess() < m.frontDistanceStopThresh.Get()) {
		//		log.Println("Reached wall in front")
		//		break
		//	}
		//}

		cornerSensorOffset := m.cornerSensorOffset.Get()
		cornerSensorAngleOffset := float64(m.cornerSensorAngleOffset.Get())
		frontLeftHorizEstMMs := float64(frontLeft.BestGuess()+cornerSensorOffset) *
			(100.0 + cornerSensorAngleOffset) / 100 / 1.8
		frontRightHorizEstMMs := float64(frontRight.BestGuess()+cornerSensorOffset) *
			(100.0 + cornerSensorAngleOffset) / 100 / 1.8

		var frontTnError, sideTnError, tnError, rotError float64
		var sideGuess, frontGuess bool
		if sideLeft.IsGood() && sideRight.IsGood() {
			leftGuess := float64(sideLeft.BestGuess())
			rightGuess := float64(sideRight.BestGuess())
			sideTnError = float64(leftGuess - rightGuess)
			sideGuess = true
			tnError = sideTnError
		}
		if frontLeft.IsGood() && frontRight.IsGood() {
			frontTnError = frontLeftHorizEstMMs - frontRightHorizEstMMs
			frontGuess = true
			tnError += frontTnError
			if sideGuess {
				tnError /= 2
			}
		}

		if frontGuess && sideGuess {
			rotError = frontTnError - sideTnError
		}

		fmt.Printf("Translation error %.1f %.1f ->  %.1f Rotation error %.1f\n", frontTnError, sideTnError, tnError, rotError)

		// Only calculate integral and differential terms if time delay was sane.
		timeSinceLastReading := readingTime.Sub(lastReadingTime)
		var dTnError, dRotError float64
		if timeSinceLastReading < 200*time.Millisecond {
			timeSinceLastSecs := timeSinceLastReading.Seconds()
			tnErrorInt += timeSinceLastSecs * tnError
			rotErrorInt += timeSinceLastSecs * rotError
			if timeSinceLastReading > 0 && lastTnError != 0 {
				dTnError = (tnError - lastTnError) / timeSinceLastSecs
				dRotError = (rotError - lastRotError) / timeSinceLastSecs
			}
		}

		tnKp := float64(m.tnKp.Get()) / 1000
		tnKi := float64(m.tnKi.Get()) / 10000
		tnKd := float64(m.tnKd.Get()) / 10000
		rotKp := float64(m.rotKp.Get()) / 1000
		rotKi := float64(m.rotKi.Get()) / 10000
		rotKd := float64(m.rotKd.Get()) / 10000

		scaledTxErr := tnKp*tnError + tnKi*tnErrorInt + tnKd*dTnError
		scaledRotErr := rotKp*rotError + rotKi*rotErrorInt + rotKd*dRotError
		_ = scaledRotErr
		fmt.Println("TX P", tnError, "I", tnErrorInt, "D", dTnError)

		if -80 < tnError && tnError < 80 &&
			-80 < rotError && rotError < 80 {
			// Only speed up if we're in the middle...
			if speed < float64(m.topSpeed.Get()) {
				speed += float64(m.speedRampUp.Get())
			}
		} else if -100 > tnError || tnError > 100 ||
			-40 > rotError || rotError > 40 {
			// Slow down if we get too close to the wall
			if speed > baseSpeed {
				speed -= float64(m.speedRampDown.Get())
			}
		}
		//
		//fl := clamp(speed-scaledTxErr-scaledRotErr, speed*2)
		//fr := clamp(speed+scaledTxErr+scaledRotErr, speed*2)
		//bl := clamp(speed+scaledTxErr-scaledRotErr, speed*2)
		//br := clamp(speed-scaledTxErr+scaledRotErr, speed*2)
		//
		//fmt.Printf("%v Speeds: FL=%d FR=%d BL=%d BR=%d\n", timeSinceLastReading, fl, fr, bl, br)
		//m.Propeller.SetMotorSpeeds(fl, fr, bl, br)

		m.headingHolder.SetControlInputs(0, 1, -scaledTxErr/127)

		lastReadingTime = readingTime
		lastTnError = tnError
		lastRotError = rotError
	}

	fmt.Println("Stopping...")
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
	m.sequenceWG.Wait()
	m.running = false
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
