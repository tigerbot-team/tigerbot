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
	Stop           bool
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

var CheckAssumptions = false

func Displacements(log Log, normalizedAngle, bl, br, fl, fr float64) (ahead, left float64) {
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
	const k = float64(1.044)
	use := func(string) {}  // no-op
	expect := func(bool) {} // no-op
	if CheckAssumptions {
		use = func(choice string) {
			if choice == "blminusfr" {
				if math.Abs(blminusfr) < 0.9*math.Abs(flminusbr) {
					panic("wrong blminusfr choice")
				}
			} else if choice == "flminusbr" {
				if 0.9*math.Abs(blminusfr) > math.Abs(flminusbr) {
					panic("wrong flminusbr choice")
				}
			} else {
				panic("unsupported choice")
			}
		}
		expect = func(b bool) {
			if !b {
				panic("wrong expectation")
			}
		}
	}
	var F, S float64
	if normalizedAngle > 135 {
		//
		// --------------+
		//              /|
		//             / |
		//            /  |
		//           / T |
		//          /    |
		//         /     | -F
		//        /      |
		//       /       |
		//      /        |
		//     /         |
		//    -----------|
		//         S/k
		//
		// T = 180 - normalizedAngle
		// tan T = -S/kF
		// flminusbr = 2(F-S) = 2F(1+k tan T)
		use("flminusbr")
		expect(flminusbr <= 0)
		tan := math.Tan((180 - normalizedAngle) * RADIANS_PER_DEGREE)
		F = 0.5 * flminusbr / (1 + k*tan) // was: 0.5 * flminusbr / (1 - k*tan)
		expect(F <= 0)
		S = -k * F * tan // was: S = k * F * tan
		expect(S >= 0)
	} else if normalizedAngle > 90 {
		//
		// --------------+
		//            T /|
		//             / |
		//            /  |
		//           /   |
		//          /    |
		//         /     | -F
		//        /      |
		//       /       |
		//      /        |
		//     /         |
		//    -----------|
		//         S/k
		//
		// T = normalizedAngle - 90
		// tan T = -kF/S
		// flminusbr = 2(F-S) = 2S(-(tan T) / k - 1)
		use("flminusbr")
		expect(flminusbr <= 0)
		tan := math.Tan((normalizedAngle - 90) * RADIANS_PER_DEGREE)
		S = -0.5 * flminusbr / (1 + tan/k) // was: -0.5 * flminusbr / (1 - k*tan)
		expect(S >= 0)
		F = -S * tan / k // was: S * tan / k
		expect(F <= 0)
	} else if normalizedAngle > 45 {
		//
		//        S/k
		//    -----------|
		//     \         |
		//      \        |
		//       \       |
		//        \      |
		//         \     |
		//          \    | F
		//           \   |
		//            \  |
		//             \ |
		//           T  \|
		// --------------+
		//
		// T = 90 - normalizedAngle
		// tan T = kF/S
		// blminusfr = 2(F+S) = 2S(1 + (tan T) / k)
		use("blminusfr")
		expect(blminusfr >= 0)
		tan := math.Tan((90 - normalizedAngle) * RADIANS_PER_DEGREE)
		S = 0.5 * blminusfr / (1 + tan/k) // was: 0.5 * blminusfr / (1 + k*tan)
		expect(S >= 0)
		F = S * tan / k // correct
		expect(F >= 0)
	} else if normalizedAngle > 0 {
		//
		//        S/k
		//    -----------|
		//     \         |
		//      \        |
		//       \       |
		//        \      | F
		//         \     |
		//          \    |
		//           \ T |
		//            \  |
		//             \ |
		//              \|
		// --------------+
		//
		// T = normalizedAngle
		// tan T = S/kF
		// blminusfr = 2(F+S) = 2F(1 + k tan T)
		use("blminusfr")
		expect(blminusfr >= 0)
		tan := math.Tan(normalizedAngle * RADIANS_PER_DEGREE)
		F = 0.5 * blminusfr / (1 + k*tan) // correct
		expect(F >= 0)
		S = F * k * tan // correct
		expect(S >= 0)
	} else if normalizedAngle > -45 {
		//
		//        -S/k
		//    |----------
		//    |         /
		//    |        /
		//    |       /
		//  F |      /
		//    |     /
		//    |    /
		//    | T /
		//    |  /
		//    | /
		//    |/
		//    +-------------
		//
		// T = - normalizedAngle
		// tan T = -S/kF
		// flminusbr = 2(F-S) = 2F(1 + k tan T)
		use("flminusbr")
		expect(flminusbr >= 0)
		tan := math.Tan(-normalizedAngle * RADIANS_PER_DEGREE)
		F = 0.5 * flminusbr / (1 + k*tan) // was: 0.5 * flminusbr / (1 - k*tan)
		expect(F >= 0)
		S = -F * k * tan // was: k * F * tan
		expect(S <= 0)
	} else if normalizedAngle > -90 {
		//
		//        -S/k
		//    |----------
		//    |         /
		//    |        /
		//    |       /
		//  F |      /
		//    |     /
		//    |    /
		//    |   /
		//    |  /
		//    | /
		//    |/ T
		//    +-------------
		//
		// T = 90 + normalizedAngle
		// tan T = -kF/S
		// flminusbr = 2(F-S) = 2S(- tan T / k - 1)
		use("flminusbr")
		expect(flminusbr >= 0)
		tan := math.Tan((90 + normalizedAngle) * RADIANS_PER_DEGREE)
		S = -0.5 * flminusbr / (1 + tan/k) // was: -0.5 * flminusbr / (1 - k*tan)
		expect(S <= 0)
		F = -S * tan / k // was: S * tan / k
		expect(F >= 0)
	} else if normalizedAngle > -135 {
		//
		//    +-------------
		//    |\ T
		//    | \
		//    |  \
		//    |   \
		//    |    \
		//    |     \
		// -F |      \
		//    |       \
		//    |        \
		//    |         \
		//    |          \
		//    |-----------
		//        -S/k
		//
		// T = - normalizedAngle - 90
		// tan T = kF/S
		// blminusfr = 2(F+S) = 2S(tan T / k + 1)
		use("blminusfr")
		expect(blminusfr <= 0)
		tan := math.Tan((-normalizedAngle - 90) * RADIANS_PER_DEGREE)
		S = 0.5 * blminusfr / (1 + tan/k) // was: 0.5 * blminusfr / (1 + k*tan)
		expect(S <= 0)
		F = S * tan / k // correct
		expect(F <= 0)
	} else {
		//
		//    +-------------
		//    |\
		//    | \
		//    |  \
		//    | T \
		//    |    \
		//    |     \
		// -F |      \
		//    |       \
		//    |        \
		//    |         \
		//    |          \
		//    |-----------
		//        -S/k
		//
		// T = 180 + normalizedAngle
		// tan T = S/kF
		// blminusfr = 2(F+S) = 2F(1 + k tan T)
		use("blminusfr")
		expect(blminusfr <= 0)
		tan := math.Tan((180 + normalizedAngle) * RADIANS_PER_DEGREE)
		F = 0.5 * blminusfr / (1 + k*tan) // correct
		expect(S <= 0)
		S = F * k * tan // correct
		expect(F <= 0)
	}

	log("F %v S %v", F, S)

	return chassis.WheelCircumMM * F, chassis.WheelCircumMM * S / k
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
	m.log("leftDisplacement %v", leftDisplacement)

	dx, dy := AbsoluteDeltas(position.Heading, aheadDisplacement, leftDisplacement)
	position.X += dx
	position.Y += dy
	m.log("position after movement %#v", *position)
}

// Given a `botHeading` (CCW relative to +tive X axis) and distances
// that the bot wants to move `ahead` and `left`, calculate the DX and
// DY relative to the arena coordinate system.
func AbsoluteDeltas(botHeading, ahead, left float64) (dx, dy float64) {
	sin := math.Sin(botHeading * RADIANS_PER_DEGREE)
	cos := math.Cos(botHeading * RADIANS_PER_DEGREE)
	dx = ahead*cos - left*sin
	dy = ahead*sin + left*cos
	return
}

func (m *ChallengeMode) StartMotion(
	ctx context.Context,
	hh hardware.HeadingAbsolute,
	current, target *Position,
	moveTime time.Duration) {

	if target.Stop {
		hh.SetThrottle(0)
	}

	if target.Heading != current.Heading {
		m.log("Heading change %v -> %v", current.Heading, target.Heading)
		hh.SetHeading(calibratedXHeading + target.Heading*PositiveAnglesAnticlockwise)
		hh.Wait(ctx)
		current.Heading = target.Heading
	}

	if target.Stop {
		return
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

	throttle := dist * 0.95 / moveTime.Seconds()
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

type obs2 struct {
	bl, br, fl, fr, angle, oldAhead, oldLeft float64
}

func TestDisplace2() {
	var observations = []obs2{
		// Values from log of Minesweeper test 18/4 evening.
		{0.62109375, 0.015625, 0, -0.60546875, 45, 65.98218731453308, 65.98218731453306},
		{0.625, 0, -0.03125, -0.67578125, 45, 69.97474005012583, 69.97474005012582},
		{0.53515625, -0.00390625, -0.02734375, -0.578125, 45, 59.88829103389148, 59.88829103389146},
		{0.46484375, -0.390625, 0.4609375, -0.3828125, 1.1213252548714081e-14, 93.63418729253956, 1.83249760842351e-14},
		{0.390625, -0.44140625, 0.3984375, -0.4453125, 7.105427357601002e-15, 92.34564343071561, 1.1452069712011759e-14},
		{0, 0, 0, 0, 7.105427357601002e-15, 0, 0},
		{0, 0, 0, 0, 7.105427357601002e-15, 0, 0},
		{0.38671875, -0.4609375, 0.37890625, -0.47265625, 0, 94.49321653375549, 0},
		{0.4609375, -0.37890625, 0.46875, -0.37890625, 0, 93.20467267193156, 0},
		{0.4140625, -0.4296875, 0.421875, -0.43359375, 0, 93.63418729253954, 0},
		{0.4375, -0.39453125, 0.43359375, -0.40625, 0, 92.77515805132357, 0},
		{0.125, -0.12109375, 0.125, -0.125, 2.842170943040401e-14, 27.488935718910678, 1.3635952773372135e-14},
		{0.44140625, -0.3984375, 0.4453125, -0.41015625, -1.4210854715202004e-14, 93.63418729253951, 2.322373206714942e-14},
		{-0.34375, -0.19921875, 0.2421875, 0.38671875, -105.53105000000002, -16.57238124876509, -59.632566211626575},
		{-0.1015625, -0.06640625, 0.03125, 0.0703125, -105.53105000000002, -3.8993838232388445, -14.031192049794488},
		{0, 0, 0, 0, -105.53105000000002, 0, 0},
		{0, 0, 0, 0, -105.53105000000002, 0, 0},
		{0, 0, 0, 0, -105.53105000000002, 0, 0},
		{0, 0, 0, 0, -105.53105000000002, 0, 0},
		{0, 0, 0, 0, -105.53105000000002, 0, 0},
		{0.41796875, -0.43359375, 0.421875, -0.44140625, -0, 94.49321653375549, -0},
		{0.421875, -0.41015625, 0.41796875, -0.42578125, -0, 93.20467267193156, -0},
		{0.390625, -0.453125, 0.3828125, -0.46484375, -0, 94.06370191314751, -0},
		{0.421875, -0.4296875, 0.4296875, -0.43359375, -0, 94.49321653375549, -0},
		{0.39453125, -0.43359375, 0.40234375, -0.44140625, -0, 91.91612881010762, -0},
		{0.421875, -0.4296875, 0.42578125, -0.4375, -0, 94.49321653375549, -0},
		{0.45703125, -0.38671875, 0.4609375, -0.390625, -0, 93.20467267193156, -0},
		{0.11328125, -0.11328125, 0.109375, -0.11328125, -0, 24.91184799526281, -0},
		{-0.08203125, 0.3359375, -0.32421875, 0.06640625, 147.32839999999987, -219.6344671568, -140.8491343621183},
		{0.55859375, 0.015625, -0.1015625, -0.65625, 46.072294610368886, 61.450600512724755, 63.794851809046435},
		{0.6796875, 0.05859375, 0.01171875, -0.625, 46.072294610368886, 65.99517868569153, 68.51279904894376},
		{0.33203125, -0.0078125, -0.03515625, -0.36328125, 46.072294610368886, 35.171083251655965, 36.51280907398799},
		{0, 0, 0, 0, 46.072294610368886, 0, 0},
		{0, 0, 0, 0, 46.072294610368886, 0, 0},
		{0, 0, 0, 0, 46.072294610368886, 0, 0},
		{0, 0, 0, 0, 46.072294610368886, 0, 0},
		{0.3203125, -0.5078125, 0.3203125, -0.5078125, -0, 91.05709956889166, -0},
		{0.42578125, -0.421875, 0.421875, -0.43359375, -0, 94.49321653375549, -0},
		{0.4296875, -0.4296875, 0.43359375, -0.42578125, -0, 94.92273115436348, -0},
		{0.41796875, -0.41796875, 0.42578125, -0.4296875, -0, 93.20467267193156, -0},
		{0.41015625, -0.4375, 0.421875, -0.4375, -0, 94.49321653375549, -0},
		{0.4296875, -0.41015625, 0.4296875, -0.42578125, -0, 94.06370191314751, -0},
		{0.08984375, -0.09375, 0.09375, -0.09375, 1.1368683772161603e-13, 20.61670178918306, 4.0907858320116505e-14},
		{-0.19140625, 0.03125, 0.0078125, 0.1953125, -136.70600000000013, -21.436653113067024, -20.19664207578827},
		{0, 0, 0, 0, -136.70600000000013, 0, 0},
		{0.390625, -0.44140625, 0.39453125, -0.44140625, -0, 91.91612881010762, -0},
		{0.41796875, -0.41015625, 0.41015625, -0.42578125, -0, 92.77515805132357, -0},
		{0.41015625, -0.421875, 0.4140625, -0.42578125, -0, 91.91612881010762, -0},
		{0.4296875, -0.40625, 0.43359375, -0.41015625, -0, 92.3456434307156, -0},
		{0.40234375, -0.4375, 0.40625, -0.44921875, -0, 93.63418729253954, -0},
		{0.08203125, -0.08203125, 0.078125, -0.07421875, -0, 17.61009944492716, -0},
		{-0.015625, 0.640625, -0.5859375, 0.01953125, 137.3704499999999, -3458.2680421638584, -3183.3337656062763},
		{0.578125, 0.05859375, -0.10546875, -0.64453125, 47.172562387618655, 60.6595830552214, 65.44349897837337},
		{0.66015625, 0.046875, -0.02734375, -0.64453125, 47.172562387618655, 64.72939533688162, 69.83427686510129},
		{0.625, 0.0234375, -0.0546875, -0.6640625, 47.172562387618655, 63.95419299751777, 68.99793822001024},
		{0.609375, 0.0078125, -0.0703125, -0.703125, 47.172562387618655, 65.11699650656354, 70.2524461876468},
		{0.64453125, 0.02734375, -0.03125, -0.66015625, 47.172562387618655, 64.72939533688162, 69.83427686510129},
		{0.625, 0.0234375, -0.07421875, -0.68359375, 47.172562387618655, 64.92319592172257, 70.04336152637404},
	}

	for _, o := range observations {
		fmt.Println("")
		ahead, left := Displacements(log.Printf, o.angle, o.bl, o.br, o.fl, o.fr)
		fmt.Printf("%v -> ahead %v left %v\n", o, ahead, left)
	}
}
