package lavapalava

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
	return "LAVAPALAVA"
}

func (c *challenge) Start(log challengemode.Log) *challengemode.Position {
	c.log = log
	panic("implement me!")
}

func (c *challenge) Iterate(
	position *challengemode.Position,
	timeSinceStart time.Duration,
) (
	bool, // at end
	*challengemode.Position, // next target
	time.Duration, // move time
) {
	panic("implement me!")
}
