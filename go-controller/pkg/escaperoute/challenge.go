package escaperoute

import (
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

const (
	// Total arena sizes (internal).
	dxTotal = float64(1809)
	dyTotal = float64(3025)

	// Y size in the initial stretch.
	dyInitial = float64(589)
	dxInitial = float64(504) // estimate from diagram

	// Y size of the coloured blocks.
	dyBlue  = float64(605) // estimate from diagram
	dyGreen = float64(405) // estimate: 2/3 of blue
	dyRed   = float64(200) // estimate: 1/3 of blue

	// Y offset from each block to the next.
	dyGap = float64(613) // estimate: (total - (blue + green + red + initial)) / 2

	// X size of the finish line.
	dxFinish = float64(793)

	// X size of the coloured blocks.
	dxBlock = float64(1016) // dxTotal - dxFinish
)

type stage int

const (
	INIT stage = iota
	PAST_LEFT_EDGE_OF_FIRST_BLOCK
	ADVANCED_TO_FACE_SECOND_BLOCK
	PAST_RIGHT_EDGE_OF_SECOND_BLOCK
	ADVANCED_TO_FACE_THIRD_BLOCK
	PAST_LEFT_EDGE_OF_THIRD_BLOCK
	ADVANCED_PAST_EXIT
)

func (s stage) String() string {
	switch s {
	case INIT:
		return "INIT"
	case PAST_LEFT_EDGE_OF_FIRST_BLOCK:
		return "PAST_LEFT_EDGE_OF_FIRST_BLOCK"
	case ADVANCED_TO_FACE_SECOND_BLOCK:
		return "ADVANCED_TO_FACE_SECOND_BLOCK"
	case PAST_RIGHT_EDGE_OF_SECOND_BLOCK:
		return "PAST_RIGHT_EDGE_OF_SECOND_BLOCK"
	case ADVANCED_TO_FACE_THIRD_BLOCK:
		return "ADVANCED_TO_FACE_THIRD_BLOCK"
	case PAST_LEFT_EDGE_OF_THIRD_BLOCK:
		return "PAST_LEFT_EDGE_OF_THIRD_BLOCK"
	case ADVANCED_PAST_EXIT:
		return "ADVANCED_PAST_EXIT"
	}
	return "???"
}

type challenge struct {
	log   challengemode.Log
	stage stage
}

func New() challengemode.Challenge {
	return &challenge{}
}

func (c *challenge) Name() string {
	return "ESCAPEROUTE"
}

func (c *challenge) Start(log challengemode.Log) *challengemode.Position {
	c.log = log
	c.stage = INIT
	return &challengemode.Position{
		X: dxTotal - (dxInitial / 2),
		Y: dyInitial / 2,
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
	c.log("Stage = %v", c.stage)
nextStage:
	switch c.stage {
	case INIT:
		target = &challengemode.Position{
			X:       (dxTotal - dxBlock) / 2,
			Y:       dyInitial / 2,
			Heading: 90,
		}
		if challengemode.TargetReached(target, position) {
			c.stage = PAST_LEFT_EDGE_OF_FIRST_BLOCK
			goto nextStage
		}
		return false, target, time.Second
	case PAST_LEFT_EDGE_OF_FIRST_BLOCK:

	}
}
