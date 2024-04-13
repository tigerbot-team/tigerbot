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
	FACING_FIRST_BLOCK
	FACING_FIRST_EDGE
	PAST_FIRST_EDGE
	FACING_SECOND_BLOCK
	FACING_SECOND_EDGE
	PAST_SECOND_EDGE
	FACING_THIRD_BLOCK
	FACING_THIRD_EDGE
	PAST_THIRD_EDGE
	ADVANCED_PAST_EXIT
)

type blockColour int

const (
	BLUE blockColour = iota
	GREEN
	RED
)

var dyBlock [3]float64

func init() {
	dyBlock[BLUE] = dyBlue
	dyBlock[GREEN] = dyGreen
	dyBlock[RED] = dyRed
}

func (s stage) String() string {
	switch s {
	case INIT:
		return "INIT"
	case FACING_FIRST_BLOCK:
		return "FACING_FIRST_BLOCK"
	case FACING_FIRST_EDGE:
		return "FACING_FIRST_EDGE"
	case PAST_FIRST_EDGE:
		return "PAST_FIRST_EDGE"
	case FACING_SECOND_BLOCK:
		return "FACING_SECOND_BLOCK"
	case FACING_SECOND_EDGE:
		return "FACING_SECOND_EDGE"
	case PAST_SECOND_EDGE:
		return "PAST_SECOND_EDGE"
	case FACING_THIRD_BLOCK:
		return "FACING_THIRD_BLOCK"
	case FACING_THIRD_EDGE:
		return "FACING_THIRD_EDGE"
	case PAST_THIRD_EDGE:
		return "PAST_THIRD_EDGE"
	case ADVANCED_PAST_EXIT:
		return "ADVANCED_PAST_EXIT"
	}
	return "???"
}

type challenge struct {
	log             challengemode.Log
	stage           stage
	blockDone       map[blockColour]bool
	thisBlockColour blockColour
	xTarget         float64
	yTarget         float64
}

func New() challengemode.Challenge {
	return &challenge{
		blockDone: map[blockColour]bool{},
	}
}

func (c *challenge) Name() string {
	return "ESCAPEROUTE"
}

func (c *challenge) Start(log challengemode.Log) (*challengemode.Position, bool) {
	c.log = log
	c.stage = INIT
	c.blockDone[BLUE] = false
	c.blockDone[GREEN] = false
	c.blockDone[RED] = false

	// Assume we're initially positioned in the middle of the
	// start box.  Also make this the initial target - so that we
	// will rotate if needed, but not try to displace.
	c.xTarget = dxTotal - (dxInitial / 2)
	c.yTarget = dyInitial / 2
	return &challengemode.Position{
		X: c.xTarget,
		Y: c.yTarget,
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
	c.log("Stage = %v", c.stage)
	for {
		// Return the current target, if we haven't yet
		// reached it.
		target := &challengemode.Position{
			X:       c.xTarget,
			Y:       c.yTarget,
			Heading: 90,
		}
		if !challengemode.TargetReached(target, position) {
			c.log("Target (%v, %v, %v) not yet reached", target.X, target.Y, target.Heading)
			return false, target, time.Second
		}
		c.log("Target (%v, %v, %v) reached", target.X, target.Y, target.Heading)

		// Current target reached, so transition to next
		// stage.
		c.stage += 1
		c.log("Stage => %v", c.stage)
		switch c.stage {
		case FACING_FIRST_BLOCK:
			// Use camera to identify block colour.
			c.thisBlockColour = c.IdentifyFacingBlockColour()

			// Move to where the left edge of the block
			// should be.
			c.xTarget = dxTotal - dxBlock
			c.yTarget = dyInitial / 2
		case FACING_FIRST_EDGE:
			// Use camera to check believed position against block
			// edge.  Offset value is +tive if the edge is to the
			// right of the camera centreline and -tive if the
			// edge is to the left.
			c.AdjustPositionByBlockEdge(position)

			// Move to position for driving past the block.
			c.xTarget = (dxTotal - dxBlock) / 2
		case PAST_FIRST_EDGE:
			// Advance to Y position facing next block.
			c.yTarget = dyInitial + dyBlock[c.thisBlockColour] + dyGap/2
		case FACING_SECOND_BLOCK:
			// Use camera to identify block colour.
			c.thisBlockColour = c.IdentifyFacingBlockColour()

			// Move to where the right edge of the block should be.
			c.xTarget = dxBlock
		case FACING_SECOND_EDGE:
			// Use camera to check believed position against block
			// edge.  Offset value is +tive if the edge is to the
			// right of the camera centreline and -tive if the
			// edge is to the left.
			c.AdjustPositionByBlockEdge(position)

			// Move to position for driving past the block.
			c.xTarget = (dxTotal + dxBlock) / 2
		case PAST_SECOND_EDGE:
			// Advance to Y position facing next block.
			c.yTarget += dyBlock[c.thisBlockColour] + dyGap
		case FACING_THIRD_BLOCK:
			// Move to where the left edge of the block should be.
			c.xTarget = dxTotal - dxBlock
		case FACING_THIRD_EDGE:
			// Use camera to check believed position against block
			// edge.  Offset value is +tive if the edge is to the
			// right of the camera centreline and -tive if the
			// edge is to the left.
			c.AdjustPositionByBlockEdge(position)

			// Move to position for driving past the block.
			c.xTarget = (dxTotal - dxBlock) / 2
		case PAST_THIRD_EDGE:
			// Move past exit.
			c.yTarget = dyTotal
		case ADVANCED_PAST_EXIT:
			return true, nil, 0
		}
	}
}

func (c *challenge) IdentifyFacingBlockColour() blockColour {
	rsp, err := challengemode.CameraExecute("id-block-colour")
	if err != nil {
		c.log("IdentifyFacingBlockColour camera err=%v", err)
	}
	switch rsp {
	case "blue":
		return BLUE
	case "green":
		return GREEN
	default:
		return RED
	}
}

func (c *challenge) AdjustPositionByBlockEdge(position *challengemode.Position) {

	// Commenting this out so that we have a potentially complete
	// candidate code for escape route.  Can add it back in if we
	// have time to do that.

	//blockEdgeOffset := c.GetBlockEdgeOffset()
	//position.X -= blockEdgeOffset * adjustmentMMPerOffset
	//panic("implement me")
}
