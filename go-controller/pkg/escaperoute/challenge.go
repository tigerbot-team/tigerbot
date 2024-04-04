package escaperoute

import (
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

type challenge struct{}

func New() challengemode.Challenge {
	return &challenge{}
}

func (c *challenge) Name() string {
	return "ESCAPEROUTE"
}

func (c *challenge) Start() *challengemode.Position {
	panic("implement me!")
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
	panic("implement me!")
}
