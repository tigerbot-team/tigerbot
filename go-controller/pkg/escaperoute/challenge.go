package escaperoute

import (
	"strconv"
	"strings"
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
	PAST_FIRST_EDGE
	FACING_SECOND_BLOCK
	PAST_SECOND_EDGE
	FACING_THIRD_BLOCK
	PAST_THIRD_EDGE
	ADVANCED_PAST_EXIT
)

type blockColour int

const (
	BLUE blockColour = iota
	GREEN
	RED
)

func (c blockColour) String() string {
	switch c {
	case BLUE:
		return "BLUE"
	case GREEN:
		return "GREEN"
	case RED:
		return "RED"
	}
	return "???"
}

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
	case PAST_FIRST_EDGE:
		return "PAST_FIRST_EDGE"
	case FACING_SECOND_BLOCK:
		return "FACING_SECOND_BLOCK"
	case PAST_SECOND_EDGE:
		return "PAST_SECOND_EDGE"
	case FACING_THIRD_BLOCK:
		return "FACING_THIRD_BLOCK"
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
	headingTarget   float64
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
	testModeCalls = 0

	// Assume we're initially positioned in the middle of the
	// start box.  Also make this the initial target - so that we
	// will rotate if needed, but not try to displace.
	c.xTarget = dxTotal - (dxInitial / 2)
	c.yTarget = dyInitial / 2
	c.headingTarget = 90
	return &challengemode.Position{
		X:              c.xTarget,
		Y:              c.yTarget,
		Heading:        90,
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
	c.log("Stage = %v", c.stage)
	for {
		// Return the current target, if we haven't yet
		// reached it.
		target := &challengemode.Position{
			X:       c.xTarget,
			Y:       c.yTarget,
			Heading: c.headingTarget,
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
			c.log("blockColour is %v", c.thisBlockColour)
			// Move to where the left edge of the block
			// should be.
			c.xTarget = (dxTotal - dxBlock) / 2
			c.yTarget = dyInitial / 2
			c.headingTarget = 180
		case PAST_FIRST_EDGE:
			// Advance to Y position facing next block.
			c.yTarget = dyInitial + dyBlock[c.thisBlockColour] + dyGap/2
			c.headingTarget = 90
		case FACING_SECOND_BLOCK:
			// Use camera to identify block colour.
			c.thisBlockColour = c.IdentifyFacingBlockColour()
			c.log("blockColour is %v", c.thisBlockColour)

			// Move to where the right edge of the block should be.
			c.xTarget = (dxTotal + dxBlock) / 2
			c.headingTarget = 0
		case PAST_SECOND_EDGE:
			// Advance to Y position facing next block.
			c.yTarget += dyBlock[c.thisBlockColour] + dyGap
			c.headingTarget = 90
		case FACING_THIRD_BLOCK:
			// Move to where the left edge of the block should be.
			c.xTarget = (dxTotal - dxBlock) / 2
			c.headingTarget = 180
		case PAST_THIRD_EDGE:
			// Move past exit.
			c.yTarget = dyTotal + 1000
			c.headingTarget = 90
		case ADVANCED_PAST_EXIT:
			return true, nil, 0
		}
	}
}

var testMode bool = false
var testModeCalls int = 0

func (c *challenge) IdentifyFacingBlockColour() blockColour {
	if testMode {
		testModeCalls++
		switch testModeCalls {
		case 1:
			return BLUE
		case 2:
			return RED
		default:
			return GREEN
		}
	}
	rsp, err := challengemode.CameraExecute(c.log, "id-block-colour")
	if err != nil {
		c.log("IdentifyFacingBlockColour camera err=%v", err)
	}
	rspWords := strings.Split(rsp, " ")
	var bestArea float64 = 0
	var bestColour blockColour
	for i := 0; i < len(rspWords); i += 2 {
		var col blockColour
		switch rspWords[i] {
		case "blue":
			col = BLUE
		case "green":
			col = GREEN
		case "red":
			col = RED
		default:
			c.log("unexpected colour '%v'", rspWords[i])
			continue
		}
		area, err := strconv.ParseFloat(rspWords[i+1], 64)
		if err != nil {
			c.log("error parsing '%v': %v", rspWords[i+1], err)
			continue
		}
		if area > bestArea && !c.blockDone[col] {
			bestArea = area
			bestColour = col
		}
	}

	c.blockDone[bestColour] = true
	return bestColour
}

func (c *challenge) AdjustPositionByBlockEdge(position *challengemode.Position) {

	// Commenting this out so that we have a potentially complete
	// candidate code for escape route.  Can add it back in if we
	// have time to do that.

	//blockEdgeOffset := c.GetBlockEdgeOffset()
	//position.X -= blockEdgeOffset * adjustmentMMPerOffset
	//panic("implement me")
}

func (c *challenge) SpeedMMPerS() float64 {
	return 200
}
