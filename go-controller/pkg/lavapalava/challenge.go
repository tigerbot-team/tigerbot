package lavapalava

import (
	"math"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

const (
	dxWidth          = float64(550)
	dyLength         = float64(7000)
	targetStepLength = float64(1000)
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

func (c *challenge) Start(log challengemode.Log) (*challengemode.Position, bool) {
	c.log = log

	// Assume we're initially positioned in the middle of the
	// bottom end of the course.  Second return value false means
	// that we don't want to switch off the motors after each
	// iteration.
	return &challengemode.Position{
		X: dxWidth / 2,
		Y: 0,
	}, false
}

func (c *challenge) Iterate(
	position *challengemode.Position,
	timeSinceStart time.Duration,
) (
	bool, // at end
	*challengemode.Position, // next target
	time.Duration, // move time
) {
	// Take a picture to work out how we should adjust our
	// heading.
	facingAdjust, movementAdjust := c.AnalyseWhiteLine()

	// Generate a target that describes both the required
	// adjustment to how the bot is facing, and the direction we
	// want the bot to displace in.
	movementHeading := 90 + movementAdjust
	target := &challengemode.Position{
		Heading: position.Heading + facingAdjust,
		X:       position.X + targetStepLength*math.Cos(movementHeading*challengemode.RADIANS_PER_DEGREE),
		Y:       position.Y + targetStepLength*math.Sin(movementHeading*challengemode.RADIANS_PER_DEGREE),
	}

	return false, target, 500 * time.Millisecond
}

func (c *challenge) AnalyseWhiteLine() (float64, float64) {
	return 0, 0
}
