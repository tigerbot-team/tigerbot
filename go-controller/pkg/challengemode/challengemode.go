package challengemode

import (
	"context"
	"log"
	"math"
	"sync"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/screen"

	"fmt"
	"sync/atomic"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/chassis"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder/angle"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/picobldc"
)

const (
	// Total bot dimensions.
	dxBot = float64(0)
	dyBot = float64(0)
)

// Absolute HH heading value that corresponds to the current arena's
// positive X direction.
//
// Shortly before each challenge, place the bot facing the arena's
// positive X direction and run the CALXHEADING mode.  This will read
// the HH heading value and store in calibratedXHeading.
var calibratedXHeading float64

// Where we believe the bot to be within the arena, and its
// orientation at that position, w.r.t. a coordinate system that makes
// sense for the arena.
type Position struct {
	X, Y           float64 // millimetres
	Heading        float64 // w.r.t. the positive X direction, +tive CCW
	HeadingIsExact bool
}

func (p *Position) String() string {
	return fmt.Sprintf("%.0f:%.0f:%.0f", p.X, p.Y, p.Heading)
}

type Challenge interface {
	Name() string

	// Set any internal state to reflect the beginning of the
	// challenge; return the initial bot position and whether to
	// stop motors after each iteration.
	Start(Log) (*Position, bool)

	// Use available sensors to update our beliefs about the arena
	// and where we are within it.
	Iterate(position *Position, timeSinceStart time.Duration) (bool, *Position, time.Duration)
}

type ChallengeMode struct {
	hw hardware.Interface

	cancel         context.CancelFunc
	startWG        sync.WaitGroup
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

	running        bool
	cancelSequence context.CancelFunc
	sequenceWG     sync.WaitGroup

	paused int32

	challenge         Challenge
	name              string
	lastThrottleAngle float64 // CCW from bot-relative straight ahead

	rotationsBeforeMotion picobldc.PerMotorVal[float64]
}

func New(hw hardware.Interface, challenge Challenge) *ChallengeMode {
	m := &ChallengeMode{
		hw:             hw,
		joystickEvents: make(chan *joystick.Event),
		challenge:      challenge,
		name:           challenge.Name(),
	}
	return m
}

func (m *ChallengeMode) Name() string {
	return m.name
}

func (m *ChallengeMode) StartupSound() string {
	// Not expecting to have sound this year, so this doesn't
	// matter.
	return "/sounds/nebulamode.wav"
}

func (m *ChallengeMode) Start(ctx context.Context) {
	m.stopWG.Add(1)
	var loopCtx context.Context
	loopCtx, m.cancel = context.WithCancel(ctx)
	go m.loop(loopCtx)
}

func (m *ChallengeMode) Stop() {
	m.cancel()
	m.stopWG.Wait()
}

func (m *ChallengeMode) loop(ctx context.Context) {
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
						m.log("Getting ready!")
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
						m.log("GO!")
						m.startWG.Done()
					}
				}
			}
		}
	}
}

func (m *ChallengeMode) startSequence() {
	if m.running {
		m.log("Already running")
		return
	}

	m.log("Starting sequence...")
	m.running = true
	atomic.StoreInt32(&m.paused, 0)

	seqCtx, cancel := context.WithCancel(context.Background())
	m.cancelSequence = cancel
	m.sequenceWG.Add(1)
	go m.runSequence(seqCtx)
}

// runSequence is a goroutine that reads from the camera and controls the motors.
func (m *ChallengeMode) runSequence(ctx context.Context) {
	defer m.sequenceWG.Done()
	defer m.log("Exiting sequence loop")
	defer m.hw.StopMotorControl()

	screen.SetEnabled(false)
	defer screen.SetEnabled(true)

	// We use the absolute heading hold mode so we can do things
	// like "turn right 90 degrees".
	hh := m.hw.StartHeadingHoldMode()

	// Get initial (believed) position - determined by the
	// challenge.  We don't have a target yet.
	position, stopEachIteration := m.challenge.Start(m.log)
	m.log("Initial position %#v stopEachIteration %v", *position, stopEachIteration)

	initialHeading := m.hw.CurrentHeading().Float()
	m.log("Initial heading %v", initialHeading)
	if position.HeadingIsExact {
		// Get initial heading as reported by the hardware.
		// Then we can store the offset from our coordinate
		// system (positive X axis = 0) to the hardware's
		// heading.
		calibratedXHeading = initialHeading - position.Heading*PositiveAnglesAnticlockwise
		m.log("Set calibratedXHeading = %v", calibratedXHeading)
	} else {
		position.Heading = (initialHeading - calibratedXHeading) / PositiveAnglesAnticlockwise
		m.log("Initial bot heading = %v", position.Heading)
	}

	// Let the user know that we're ready, then wait for the "GO" signal.
	m.hw.PlaySound("/sounds/ready.wav")
	screen.SetNotice("Ready!", screen.LevelInfo)
	m.startWG.Wait()
	screen.ClearNotice("Ready!")

	startTime := time.Now()

	iterationCount := 0

	for ctx.Err() == nil {
		iterationCount += 1

		// Note, the bot is stationary at the start of each
		// iteration of this loop.

		timeSinceStart := time.Now().Sub(startTime)

		// Challenge-specific iteration: given current
		// position, current target, and time since start of
		// challenge,
		//
		// - Use sensors to update where we think we are and
		//   what we think the arena is.
		//
		// - Decide if we've reached the end of the challenge.
		//   If not...
		//
		// - Compute the next target position that the bot
		//   should move towards, and for how long it should
		//   do that before re-evaluating.
		atEnd, target, moveTime := m.challenge.Iterate(position, timeSinceStart)
		if atEnd {
			m.log("Reached end of challenge")
			break
		}
		m.log("Iteration %v: position %#v", iterationCount, *position)
		m.log("Iteration %v: target %#v moveTime %v", iterationCount, *target, moveTime)

		// Start moving to the target position.  Note, sets
		// m.lastThrottleAngle.
		m.StartMotion(ctx, hh, position, target, moveTime)

		// Allow motion for the indicated time.
		select {
		case <-time.After(moveTime):
		case <-ctx.Done():
			m.log("Context done.")
			return
		}

		if stopEachIteration {
			// Stop moving.
			hh.SetThrottle(0)
		}

		// Update current position based on dead reckoning.
		// Note, uses m.lastThrottleAngle.
		m.UpdatePosition(position)
	}

	m.log("Run completed in %v", time.Since(startTime))

	return
}

func (m *ChallengeMode) stopSequence() {
	if !m.running {
		m.log("Not running")
		return
	}
	m.log("Stopping sequence...")

	m.cancelSequence()
	m.cancelSequence = nil
	m.sequenceWG.Wait()
	m.running = false
	atomic.StoreInt32(&m.paused, 0)

	m.hw.StopMotorControl()

	m.log("Stopped sequence...")
}

func (m *ChallengeMode) pauseOrResumeSequence() {
	if atomic.LoadInt32(&m.paused) == 1 {
		m.log("Resuming sequence...")
		atomic.StoreInt32(&m.paused, 0)
	} else {
		m.log("Pausing sequence...")
		atomic.StoreInt32(&m.paused, 1)
	}
}

func (m *ChallengeMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}

func (m *ChallengeMode) log(f string, args ...any) {
	fmt.Println(m.name + ": " + fmt.Sprintf(f, args...))
}

type Log func(string, ...any)

const RADIANS_PER_DEGREE = math.Pi / 180

const PositiveAnglesAnticlockwise float64 = 1 // Invert me if HeadingAbsolute uses the opposite sign.

func DisplacementsByTable(log Log, normalizedAngle, bl, br, fl, fr float64) (ahead, left float64) {
	rotations := math.Abs(bl) + math.Abs(br) + math.Abs(fl) + math.Abs(fr)

	// Round to the closest multiple of 5 degrees.  Note, Golang
	// rounds towards zero when converting float64 to int.
	var quantizedAngleOver5 int
	if normalizedAngle > 0 {
		quantizedAngleOver5 = int(normalizedAngle/5 + 0.5)
	} else {
		quantizedAngleOver5 = int(normalizedAngle/5 - 0.5)
	}

	// quantizedAngleOver5 is now from -36 to 36 (inclusive).
	index := (quantizedAngleOver5 + 36) % 72

	m := mmPerRotation[index]
	log("mm per rotation %v rotations %v", m, rotations)
	return rotations * m.ahead, rotations * m.left
}

func Displacements(log Log, normalizedAngle, bl, br, fl, fr float64) (ahead, left float64) {
	// To get all the signs right, use angles in the different
	// quadrants to make the tangents positive, and not if we need
	// to end up inverting the absolute X and Y displacements.
	var T, YF, XS float64
	if normalizedAngle > 90 {
		T = 180 - normalizedAngle
		YF = -1
		XS = 1
	} else if normalizedAngle >= 0 {
		T = normalizedAngle
		YF = 1
		XS = 1
	} else if normalizedAngle > -90 {
		T = -normalizedAngle
		YF = 1
		XS = -1
	} else {
		T = normalizedAngle + 180
		YF = -1
		XS = -1
	}
	log("T %v YF %v XS %v", T, YF, XS)

	// If F = forwards throttle (+tive ahead) and S = sideways
	// throttle (+tive left):
	//
	// BL = (F + S)
	// FR = - (F + S)
	//
	// BR = - (F - S)
	// FL = (F - S)
	//
	// So in theory BL = -FR and BR = -FL.  In practice those are
	// accurate to 1-2% when the absolute values are large, but we
	// see errors 20% or more when the absolute values are small.
	// Since we think we can rely on the HH _heading_, we use that
	// together with whichever pair of wheel rotations has the
	// larger absolute values.
	blminusfr := bl - fr
	flminusbr := fl - br
	log("blminusfr %v", blminusfr)
	log("flminusbr %v", flminusbr)
	var F, S float64
	const k = float64(1.044)
	if math.Abs(blminusfr) >= math.Abs(flminusbr) {
		log("use blminusfr")
		// blminusfr = 2 (F + S)
		//
		// If closer to forwards than sideways, use
		// S/kF = tan T, where k is 1.044.
		if normalizedAngle <= -135 ||
			(normalizedAngle >= -45 && normalizedAngle <= 45) ||
			normalizedAngle > 135 {
			log("closer to forwards")
			// blminusfr = 2F (1 + k tan T)
			tan := math.Tan(T * RADIANS_PER_DEGREE)
			F = 0.5 * blminusfr / (1 + k*tan)
			S = k * F * tan
		} else {
			log("closer to sideways")
			// Use Fk/S = tan (90 - T).
			//
			// blminusfr = 2S (1 + k tan (90 - T))
			tan := math.Tan((90 - T) * RADIANS_PER_DEGREE)
			S = 0.5 * blminusfr / (1 + k*tan)
			F = S * tan / k
		}
	} else {
		log("use flminusbr")
		// flminusbr = 2 (F - S)
		if normalizedAngle <= -135 ||
			(normalizedAngle >= -45 && normalizedAngle <= 45) ||
			normalizedAngle > 135 {
			log("closer to forwards")
			// flminusbr = 2F (1 - k tan T)
			tan := math.Tan(T * RADIANS_PER_DEGREE)
			F = 0.5 * flminusbr / (1 - k*tan)
			S = k * F * tan
		} else {
			// Use Fk/S = tan (90 - T).
			//
			// flminusbr = -2S (1 - k tan (90 - T))
			log("closer to sideways")
			tan := math.Tan((90 - T) * RADIANS_PER_DEGREE)
			S = -0.5 * flminusbr / (1 - k*tan)
			F = S * tan / k
		}
	}
	log("F %v S %v", F, S)

	return chassis.WheelCircumMM * YF * F, chassis.WheelCircumMM * XS * S / k
}

func (m *ChallengeMode) UpdatePosition(position *Position) {
	newRotations := m.hw.AccumulatedRotations()

	// Calculate incremental rotations of the 4 wheels
	// since before motion started.
	bl := newRotations[picobldc.BackLeft] - m.rotationsBeforeMotion[picobldc.BackLeft]
	br := newRotations[picobldc.BackRight] - m.rotationsBeforeMotion[picobldc.BackRight]
	fl := newRotations[picobldc.FrontLeft] - m.rotationsBeforeMotion[picobldc.FrontLeft]
	fr := newRotations[picobldc.FrontRight] - m.rotationsBeforeMotion[picobldc.FrontRight]
	for motor := range newRotations {
		m.rotationsBeforeMotion[motor] = newRotations[motor]
	}
	m.log("bl %v br %v fl %v fr %v", bl, br, fl, fr)

	// Mapping from wheel rotations to actual ahead and
	// sideways displacement depends on the throttle
	// angle.
	normalizedAngle := angle.FromFloat(m.lastThrottleAngle).Float()
	m.log("normalizedAngle %v", normalizedAngle)

	aheadDisplacement, leftDisplacement := Displacements(m.log, normalizedAngle, bl, br, fl, fr)
	m.log("aheadDisplacement %v", aheadDisplacement)
	m.log("leftDisplacement %v", aheadDisplacement)

	sin := math.Sin(position.Heading * RADIANS_PER_DEGREE)
	cos := math.Cos(position.Heading * RADIANS_PER_DEGREE)

	position.X += aheadDisplacement*cos - leftDisplacement*sin
	position.Y += aheadDisplacement*sin + leftDisplacement*cos
	m.log("position after movement %#v", *position)
}

func (m *ChallengeMode) StartMotion(
	ctx context.Context,
	hh hardware.HeadingAbsolute,
	current, target *Position,
	moveTime time.Duration) {
	if target.Heading != current.Heading {
		m.log("Heading change %v -> %v", current.Heading, target.Heading)
		hh.SetHeading(calibratedXHeading + target.Heading*PositiveAnglesAnticlockwise)
		hh.Wait(ctx)
	}

	// After rotating, store the current wheel rotations.
	m.rotationsBeforeMotion = m.hw.AccumulatedRotations()

	var displacementHeading float64
	if target.X == current.X {
		if target.Y > current.Y {
			displacementHeading = 90
		} else if target.Y < current.Y {
			displacementHeading = -90
		} else {
			return
		}
	} else if target.Y == current.Y {
		if target.X > current.X {
			displacementHeading = 0
		} else {
			displacementHeading = 180
		}
	} else {
		displacementHeading = math.Atan2(
			float64(target.Y-current.Y),
			float64(target.X-current.X),
		) / RADIANS_PER_DEGREE
	}
	m.log("displacementHeading %v", displacementHeading)

	dX := target.X - current.X
	dY := target.Y - current.Y
	m.log("current %v target %v", current, target)
	m.log("dx=%f dy=%f", dX, dY)
	dist := math.Sqrt((dX * dX) + (dY * dY))

	throttle := dist * 0.9 / moveTime.Seconds()
	const maxThrottle = 100
	if throttle > maxThrottle {
		throttle = maxThrottle
	}
	heading := displacementHeading - target.Heading
	m.log("Setting throttle %f heading %f", throttle, heading)
	hh.SetThrottleWithAngle(throttle, heading)
	m.lastThrottleAngle = heading
}

// Utility for challenge-specific code.
func TargetReached(currentTarget, position *Position) bool {
	const maxPositionDelta float64 = 10 // millimetres
	const maxHeadingDelta float64 = 5   // degrees
	return math.Abs(currentTarget.X-position.X) <= maxPositionDelta &&
		math.Abs(currentTarget.Y-position.Y) <= maxPositionDelta &&
		math.Abs(currentTarget.Heading-position.Heading) <= maxHeadingDelta
}

type obs struct {
	angle, fr, bl, br, fl float64
}

func TestDisplace() {
	var observations = []obs{
		// Guessed values for rectilinear motion.
		{-180, 10, -10, 10, -10},
		{-90, 10.44, -10.44, -10.44, 10.44},
		{0, -10, 10, -10, 10},
		{90, -10.44, 10.44, 10.44, -10.44},
		// From Saturday.
		{-165, 2.4296875, -2.40625, 1.09375, -1.0546875},
		{-150, 2.87109375, -2.84375, 0.3125, -0.2578125},
		{-135, 3.125, -3.08984375, -0.48828125, 0.5546875},
		{-120, 3.1484375, -3.07421875, -1.23046875, 1.328125},
		{-105, 2.984375, -2.921875, -1.94921875, 2.0234375},
		{0, -1.828125, 1.79296875, -1.73828125, 1.71875},
		{15, -2.36328125, 2.34765625, -1.0703125, 1.046875},
		{15, -2.3828125, 2.34375, -1.0859375, 1.046875},
		{30, -2.83984375, 2.83203125, -0.3046875, 0.2734375},
		{45, -3.09765625, 3.1015625, 0.50390625, -0.5390625},
		{60, -3.14453125, 3.11328125, 1.2578125, -1.31640625},
		{75, -2.97265625, 2.93359375, 1.9453125, -2.0078125},
	}

	fmt.Println(">> Using lots of trigonometry...")
	for _, o := range observations {
		ahead, left := Displacements(log.Printf, o.angle, o.bl, o.br, o.fl, o.fr)
		fmt.Printf("%v -> ahead %v left %v\n", o, ahead, left)
	}

	fmt.Println(">> Using Shaun's partially populated calibration table...")
	for _, o := range observations {
		ahead, left := DisplacementsByTable(log.Printf, o.angle, o.bl, o.br, o.fl, o.fr)
		fmt.Printf("%v -> ahead %v left %v\n", o, ahead, left)
	}
}
