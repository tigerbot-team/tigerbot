package challengemode

import (
	"context"
	"math"
	"sync"

	"fmt"
	"sync/atomic"
	"time"

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

	// We use the absolute heading hold mode so we can do things
	// like "turn right 90 degrees".
	hh := m.hw.StartHeadingHoldMode()

	// Get initial (believed) position - determined by the
	// challenge.  We don't have a target yet.
	position, stopEachIteration := m.challenge.Start(m.log)
	target := (*Position)(nil)

	initialHeading := m.hw.CurrentHeading().Float()
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
	m.startWG.Wait()

	startTime := time.Now()

	UpdatePosition := m.MakeUpdatePosition(m.hw.AccumulatedRotations())

	for {
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
			break
		}

		// Start moving to the target position.  Note, sets
		// m.lastThrottleAngle.
		m.StartMotion(hh, position, target)

		// Allow motion for the indicated time.
		time.Sleep(moveTime)

		if stopEachIteration {
			// Stop moving.
			hh.SetThrottle(0)
		}

		// Update current position based on dead reckoning.
		// Note, uses m.lastThrottleAngle.
		UpdatePosition(position, m.hw.AccumulatedRotations())
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

func (m *ChallengeMode) MakeUpdatePosition(lastRotations picobldc.PerMotorVal[float64]) func(position *Position, newRotations picobldc.PerMotorVal[float64]) {
	return func(position *Position, newRotations picobldc.PerMotorVal[float64]) {
		// Calculate an overall "rotations" number that we
		// will use to scale our calibration table.
		rotations := float64(0)
		for m := range newRotations {
			rotations += math.Abs(newRotations[m] - lastRotations[m])
			lastRotations[m] = newRotations[m]
		}

		// Mapping from wheel rotations to actual ahead and
		// sideways displacement depends on the throttle
		// angle.
		normalizedAngle := angle.FromFloat(m.lastThrottleAngle).Float()

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
		aheadDisplacement := rotations * mmPerRotation[index].ahead
		leftDisplacement := rotations * mmPerRotation[index].left

		sin := math.Sin(position.Heading * RADIANS_PER_DEGREE)
		cos := math.Cos(position.Heading * RADIANS_PER_DEGREE)

		position.X -= (aheadDisplacement*sin + leftDisplacement*cos)
		position.Y += (aheadDisplacement*cos - leftDisplacement*sin)
	}
}

func (m *ChallengeMode) StartMotion(hh hardware.HeadingAbsolute, current, target *Position) {
	if target.Heading != current.Heading {
		hh.SetHeading(calibratedXHeading + target.Heading*PositiveAnglesAnticlockwise)
		hh.Wait(context.Background())
	}
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
		displacementHeading = math.Atan2(float64(target.Y-current.Y), float64(target.X-current.X)) / RADIANS_PER_DEGREE
	}
	hh.SetThrottleWithAngle(1, displacementHeading-target.Heading)
	m.lastThrottleAngle = displacementHeading - target.Heading
}

// Utility for challenge-specific code.
func TargetReached(currentTarget, position *Position) bool {
	const maxPositionDelta float64 = 10 // millimetres
	const maxHeadingDelta float64 = 5   // degrees
	return (math.Abs(currentTarget.X-position.X) <= maxPositionDelta &&
		math.Abs(currentTarget.Y-position.Y) <= maxPositionDelta &&
		math.Abs(currentTarget.Heading-position.Heading) <= maxHeadingDelta)
}
