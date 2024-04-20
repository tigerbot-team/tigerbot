package calxheading

import (
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

type challenge struct {
	log challengemode.Log
}

func (c *challenge) SpeedMMPerS() float64 {
	return 100
}

func New() challengemode.Challenge {
	return &challenge{}
}

func (c *challenge) Name() string {
	return "CALXHEADING"
}

func (c *challenge) Start(log challengemode.Log) (*challengemode.Position, bool) {
	c.log = log

	// By definition, when this mode runs, the bot has been placed
	// facing the positive X axis of the arena.  X and Y positions
	// do not matter.  HeadingIsExact: true will cause the current
	// HH heading value to be stored in calibratedXHeading, ready
	// for the real challenge.
	return &challengemode.Position{
		Heading:        0,
		HeadingIsExact: true,
	}, true
}

func (c *challenge) Iterate(
	position *challengemode.Position,
	timeSinceStart time.Duration,
) (
	bool, // at end
	*challengemode.Position, // next target
	time.Duration, // move time
) {
	return true, nil, 0
}
