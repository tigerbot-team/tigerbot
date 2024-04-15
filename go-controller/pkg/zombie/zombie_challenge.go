package zombie

import (
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

const (
	// Total arena size.
	dxTotal = float64(1150)
	dyTotal = float64(900)

	startX  = float64(200)
	startY  = float64(200)
	targetY = float64(600)

	fixedHeading = 90
)

type stage int

const (
	INIT stage = iota
	HUNT_RIGHT
	HUNT_LEFT
)

type challenge struct {
	log           challengemode.Log
	stage         stage
	xTarget       float64
	yTarget       float64
	headingTarget float64
}

func New() challengemode.Challenge {
	return &challenge{}
}

func (c *challenge) Name() string {
	return "ZOMBIE"
}

func (c *challenge) Start(log challengemode.Log) (*challengemode.Position, bool) {
	c.log = log
	c.stage = INIT

	// Assume we're initially positioned facing straight down the course to the zombies
	// at the left of the arena.
	position := &challengemode.Position{
		X:              200,
		Y:              200,
		Heading:        fixedHeading,
		HeadingIsExact: true,
	}

	return position, true
}

func (c *challenge) Iterate(
	position *challengemode.Position,
	timeSinceStart time.Duration,
) (
	bool, // at end
	*challengemode.Position, // next target
	time.Duration, // move time
) {
	c.log("Stage = %v", c.stage)
	switch c.stage {
	case INIT:
		initialTarget := &challengemode.Position{
			X:       startX,
			Y:       targetY,
			Heading: fixedHeading,
		}

		if challengemode.TargetReached(initialTarget, position) {
			c.stage = HUNT_RIGHT
			return false, initialTarget, 0
		}

		return false, initialTarget, 1 * time.Second
	case HUNT_RIGHT:
		target := &challengemode.Position{
			X:       dxTotal - 200,
			Y:       targetY,
			Heading: fixedHeading,
		}

		if challengemode.TargetReached(target, position) {
			c.stage = HUNT_LEFT
			return false, target, 0
		}

		return false, target, 1 * time.Second
	case HUNT_LEFT:
		target := &challengemode.Position{
			X:       200,
			Y:       targetY,
			Heading: fixedHeading,
		}

		if challengemode.TargetReached(target, position) {
			c.stage = HUNT_RIGHT
			return false, target, 0
		}

		return false, target, 1 * time.Second
	}
	panic("unknown stage")
}
