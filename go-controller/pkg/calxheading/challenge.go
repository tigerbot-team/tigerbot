package calxheading

import (
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

type challenge struct {
	log challengemode.Log
}

func New() challengemode.Challenge {
	return &challenge{}
}

func (c *challenge) Name() string {
	return "CALXHEADING"
}

func (c *challenge) Start(log challengemode.Log) *challengemode.Position {
	c.log = log

	// By definition, when this mode runs, the bot has been placed
	// facing the positive X axis of the arena.  X and Y positions
	// do not matter.  HeadingIsExact: true will cause the current
	// HH heading value to be stored in calibratedXHeading, ready
	// for the real challenge.
	return &challengemode.Position{
		Heading:        0,
		HeadingIsExact: true,
	}
}

func (c *challenge) Iterate(
	position *challengemode.Position,
	target *challengemode.Position,
	timeSinceStart time.Duration,
) (
	bool,
	*challengemode.Position,
	time.Duration,
) {
	return true, nil, 0
}
