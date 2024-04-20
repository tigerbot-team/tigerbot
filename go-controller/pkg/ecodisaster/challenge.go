package ecodisaster

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

const (
	// Total arena size.
	dxTotal = float64(2200)
	dyTotal = float64(2200)

	// L/R/T/B edges of the rectangular area the barrels can be
	// in.
	xBarrelAreaL = float64(300)
	xBarrelAreaR = float64(1900)
	yBarrelAreaB = float64(300)
	yBarrelAreaT = float64(1900)

	// The drop zones.
	xGreenDropL = float64(400)
	xGreenDropR = float64(1000)
	xRedDropL   = float64(1200)
	xRedDropR   = float64(1800)
	yDropB      = float64(2000)
	yDropT      = float64(2200)

	// The starting box.
	xStartL = float64(1100) - float64(112.5)
	xStartR = float64(1100) + float64(112.5)
	yStartB = float64(200)
	yStartT = float64(600)

	// Minimum distance between the centre of the bot and any wall.
	dlBotWall = float64(200)

	// Confidence level at which we'll immediately start heading
	// to the next square, even if we haven't done a full rotation
	// of searching yet.
	immediateConfidenceThreshold = float64(11)

	// Measure of how much the confidence level could drop by once
	// we're already approaching the believed target.  In other
	// words, if the current confidence is less that this factor
	// times the confidence when we identified the target, we'll
	// decide we made a mistake and restart the search.
	allowedConfidenceDrop = float64(0.7)

	// Number of barrels that the bot can carry at once.  We'll
	// make sure that these are always all the same colour.
	BOT_CAPACITY = 3

	// Colour constants - can also be used as array indices.
	GREEN = 0 // aka clean
	RED   = 1 // aka contaminated
)

type stage int

const (
	INIT stage = iota

	// Use this state when we're searching for a barrel, or
	// approaching the barrel we plan to pick up.
	SEARCH_OR_APPROACH

	// Use this state whenever we're just moving to a new target
	// position, without actively searching, picking up or
	// dropping off.  (We might already be carrying barrels
	// though.)  The eventual target position is `moveTarget`, but
	// we will get there by moving through the safe areas around
	// the edges of the arena, and that will probably involve
	// several intermediate target positions.
	MOVING_TO_SEARCH_POSITION

	// Use this state when we're executing a drop off.
	DROPPING_OFF
)

type challenge struct {
	log   challengemode.Log
	stage stage

	// When MOVING_TO_SEARCH_POSITION, the eventual position we're
	// trying to get to.
	moveTarget challengemode.Position

	// The immediate next target position.
	intermediateTarget challengemode.Position

	// Sequence of target positions we've moved through since the
	// last safe position.
	pathFromSafe []challengemode.Position

	// Number of barrels that the bot is already carrying, and the
	// colour of those barrels (all the same).
	botLoad   int
	botColour int

	// Searching.
	searchInitialHeading float64
	bestHeading          float64
	bestConfidence       float64
	approachingTarget    bool
}

func New() challengemode.Challenge {
	return &challenge{}
}

func (c *challenge) Name() string {
	return "ECODISASTER"
}

func (c *challenge) Start(log challengemode.Log) (*challengemode.Position, bool) {
	c.log = log
	c.stage = MOVING_TO_SEARCH_POSITION
	c.log("Start")

	// Assume we're initially positioned in the middle of the
	// start box.  Don't know the heading, but calibration should
	// tell us that.
	position := &challengemode.Position{
		X: (xStartL + xStartR) / 2,
		Y: (yStartB + yStartT) / 2,
	}

	// Target the same position, so we'll immediately transition
	// to the next stage.
	c.intermediateTarget = *position
	c.moveTarget = *position

	return position, true
}

func (c *challenge) calcNextIntermediateTarget(position *challengemode.Position) {
	defer func() {
		c.log("intermediateTarget %v", c.intermediateTarget)
	}()
	lenPFS := len(c.pathFromSafe)
	if lenPFS > 0 {
		c.log("moving back along recent path")
		lastTarget := c.pathFromSafe[lenPFS-1]
		c.log("lastTarget %v", lastTarget)
		lastTargetButWithCurrentHeading := lastTarget
		lastTargetButWithCurrentHeading.Heading = position.Heading
		if challengemode.TargetReached(&lastTarget, position) {
			c.log("reached last target")
			c.pathFromSafe = c.pathFromSafe[:lenPFS-1]
			lenPFS -= 1
			if lenPFS > 0 {
				c.log("intermediate -> previous path point with current heading")
				c.intermediateTarget = c.pathFromSafe[lenPFS-1]
				c.intermediateTarget.Heading = position.Heading
				return
			}
		} else if challengemode.TargetReached(&lastTargetButWithCurrentHeading, position) {
			c.log("intermediate -> previous path point with its own heading")
			c.intermediateTarget = lastTarget
			return
		}
	}
	// Reaching here means we're back in a 'safe' place, i.e. close to one of the walls.
	if c.moveTarget.X < dxTotal/2 {
		c.log("move around the left hand side")
		if position.Y > yBarrelAreaT {
			c.log("intermediate -> real move target")
			// We should now be able to go to the real target.
			c.intermediateTarget = c.moveTarget
		} else if position.X < xBarrelAreaL {
			c.log("intermediate -> top left corner")
			// On left side wall, move to top left corner.
			c.intermediateTarget.X = dlBotWall
			c.intermediateTarget.Y = dyTotal - dlBotWall
			c.intermediateTarget.Heading = 90
		} else if position.Y < yBarrelAreaB {
			c.log("intermediate -> bottom left corner")
			// On bottom wall, move to bottom left corner.
			c.intermediateTarget.X = dlBotWall
			c.intermediateTarget.Y = dlBotWall
			c.intermediateTarget.Heading = 180
		} else {
			// We shouldn't be here, because we've already
			// covered all the safe areas above.  But if
			// we're somehow in the middle of the arena,
			// move to the closest L/B wall.
			if position.Y < position.X {
				c.log("intermediate -> bottom wall")
				c.intermediateTarget.X = position.X
				c.intermediateTarget.Y = dlBotWall
				c.intermediateTarget.Heading = -90
			} else {
				c.log("intermediate -> left wall")
				c.intermediateTarget.X = dlBotWall
				c.intermediateTarget.Y = position.Y
				c.intermediateTarget.Heading = 180
			}
		}
	} else {
		c.log("move around the right hand side")
		if position.Y > yBarrelAreaT {
			c.log("intermediate -> real move target")
			// We should now be able to go to the real target.
			c.intermediateTarget = c.moveTarget
		} else if position.X > xBarrelAreaR {
			c.log("intermediate -> top right corner")
			// On right side wall, move to top right corner.
			c.intermediateTarget.X = dxTotal - dlBotWall
			c.intermediateTarget.Y = dyTotal - dlBotWall
			c.intermediateTarget.Heading = 90
		} else if position.Y < yBarrelAreaB {
			c.log("intermediate -> bottom right corner")
			// On bottom wall, move to bottom right corner.
			c.intermediateTarget.X = dxTotal - dlBotWall
			c.intermediateTarget.Y = dlBotWall
			c.intermediateTarget.Heading = 0
		} else {
			// We shouldn't be here, because we've already
			// covered all the safe areas above.  But if
			// we're somehow in the middle of the arena,
			// move to the closest R/B wall.
			if position.Y < dxTotal-position.X {
				c.log("intermediate -> bottom wall")
				c.intermediateTarget.X = position.X
				c.intermediateTarget.Y = dlBotWall
				c.intermediateTarget.Heading = -90
			} else {
				c.log("intermediate -> right wall")
				c.intermediateTarget.X = dxTotal - dlBotWall
				c.intermediateTarget.Y = position.Y
				c.intermediateTarget.Heading = 0
			}
		}
	}
}

func (c *challenge) Iterate(
	position *challengemode.Position,
	timeSinceStart time.Duration,
) (
	bool, // at end
	*challengemode.Position, // next target
	time.Duration, // move time
) {
	var target *challengemode.Position
	c.log("Stage = %v", c.stage)
	for {
		switch c.stage {
		case MOVING_TO_SEARCH_POSITION:
			if challengemode.TargetReached(&c.moveTarget, position) {
				c.log("Target (%v, %v, %v) reached", c.moveTarget.X, c.moveTarget.Y, c.moveTarget.Heading)
				c.stage = SEARCH_OR_APPROACH
				goto stage_transition
			}
			if challengemode.TargetReached(&c.intermediateTarget, position) {
				c.log("Intermediate target (%v, %v, %v) reached", c.intermediateTarget.X, c.intermediateTarget.Y, c.intermediateTarget.Heading)
				c.calcNextIntermediateTarget(position)
			}
			return false, &c.intermediateTarget, time.Second
		case SEARCH_OR_APPROACH:
			// Try to identify a target in the direction
			// that we're currently facing - return a
			// confidence level for that, a heading
			// adjustment if we can see a red square but
			// it's slightly to the left or right, and the
			// distance left to travel in order for part
			// of the bot to be over the square.
			targetConfidence, headingAdjust, distance := c.IdentifyMine()
			c.log("targetConfidence %v headingAdjust %v distance %v", targetConfidence, headingAdjust, distance)
			calcTarget := false
			if c.approachingTarget {
				c.log("approaching target")
				// Check in case target confidence is going down.
				if targetConfidence < c.bestConfidence*allowedConfidenceDrop {
					c.log("Lost target")

					// Restart the search.
					c.stage = SEARCH_OR_APPROACH
					goto stage_transition
				}
				c.bestHeading = position.Heading + headingAdjust
				c.log("bestHeading -> %v", c.bestHeading)
			} else {
				c.log("searching for target")
				if targetConfidence > c.bestConfidence {
					c.bestConfidence = targetConfidence
					c.bestHeading = position.Heading + headingAdjust
					c.log("bestConfidence %v bestHeading %v", c.bestConfidence, c.bestHeading)
				}
				nextHeading := position.Heading + 40
				if targetConfidence < immediateConfidenceThreshold &&
					nextHeading < c.searchInitialHeading+360 {
					c.log("Not confident enough yet, nextHeading %v", nextHeading)
					// Not confident enough yet,
					// and we haven't looked round
					// the whole circle yet.
					return false, &challengemode.Position{
						X:       position.X,
						Y:       position.Y,
						Heading: nextHeading,
					}, 0
				}

				// OK, we're going to start moving
				// towards the believed target now.
				c.log("Identified target")
				c.approachingTarget = true
				calcTarget = true
			}

			if calcTarget || targetConfidence > c.bestConfidence {
				// Update target position for as long
				// as confidence increases.  It will
				// eventually decrease as the bot
				// moves onto the square, and when
				// that happens we _don't_ want to
				// recompute the target.
				c.log("Compute target position")
				c.bestConfidence = targetConfidence
				c.moveTarget.X = position.X + distance*math.Cos(c.bestHeading*challengemode.RADIANS_PER_DEGREE)
				c.moveTarget.Y = position.Y + distance*math.Sin(c.bestHeading*challengemode.RADIANS_PER_DEGREE)
				//c.headingTarget = c.bestHeading
				//c.obeyHeadingTarget = true
			}
		}

		// Return the current target, if we haven't yet
		// reached it.
		//target = &challengemode.Position{
		//	X:       c.xTarget,
		//	Y:       c.yTarget,
		//	Heading: position.Heading,
		//}
		//if c.obeyHeadingTarget {
		//	c.log("obey heading target")
		//	target.Heading = c.headingTarget
		//}
		if !challengemode.TargetReached(target, position) {
			c.log("Target (%v, %v, %v) not yet reached", target.X, target.Y, target.Heading)
			return false, target, time.Second
		}
		c.log("Target (%v, %v, %v) reached", target.X, target.Y, target.Heading)

		// Current target reached, so transition to next
		// stage.
		c.stage += 1
	stage_transition:
		c.log("Stage => %v", c.stage)
		//		switch c.stage {
		//		case POSSIBLY_UNSAFE_FOR_SEARCH:
		//			// Move to a position at least one square side
		//			// away from the walls.
		//			c.xTarget = min(max(position.X, dxInitial), dxTotal-dxInitial)
		//			c.yTarget = min(max(position.Y, dyInitial), dyTotal-dyInitial)
		//			c.obeyHeadingTarget = false
		//			c.approachingTarget = false
		//		case SAFE_FOR_SEARCH:
		//			// Beginning a search; store the initial
		//			// heading, so we don't rotate forever.
		//			c.searchInitialHeading = position.Heading
		//			c.bestConfidence = 0
		//		case ON_BOMB_SQUARE:
		//			c.log("Sit on bomb!")
		//			// Sit here for a bit more than 1 second.
		//			time.Sleep(1200 * time.Millisecond)
		//
		//			// Restart the search.
		//			c.stage = POSSIBLY_UNSAFE_FOR_SEARCH
		//			goto stage_transition
		//		}
	}
}

func (c *challenge) IdentifyMine() (confidence, headingAdjust, distance float64) {
	rsp, err := challengemode.CameraExecute(c.log, "id-mine")
	if err != nil {
		c.log("IdentifyMine camera err=%v", err)
	}
	rspWords := strings.Split(rsp, " ")
	largestContourArea, err := strconv.ParseFloat(rspWords[0], 64)
	if err != nil {
		c.log("largestContourArea '%v' err=%v", rspWords[0], err)
	}
	x, err := strconv.ParseFloat(rspWords[1], 64)
	if err != nil {
		c.log("x '%v' err=%v", rspWords[1], err)
	}
	//y, err := strconv.ParseFloat(rspWords[2], 64)
	//if err != nil {
	//	c.log("y '%v' err=%v", rspWords[2], err)
	//}

	// Based on calibration, there's a reasonably consistent
	// inverse square relation between observed area and distance:
	//
	// 2 * ln(distance) + ln(area) = 20
	//
	// where distance is in cm and area is in whatever OpenCV
	// returns for contour areas.  Inverting that...
	distance = math.Exp(10.0 - 0.5*math.Log(largestContourArea))

	// Convert from cm to mm.
	distance *= 10

	x = max(min(x, 0.8), 0.2)
	headingAdjust = 45 - 90*(x-0.2)/0.6

	// For the images I used for calibration, this gives values
	// between 10.67 and 15.17, and I think all the values >= 11
	// qualify for immediate confidence.
	confidence = math.Log(largestContourArea)

	return
}

func (c *challenge) SpeedMMPerS() float64 {
	return 100
}

type calib struct {
	picNum       int
	readyToClose bool
	aheadMM      float64
	leftMM       float64
}

// Primary barrel in all of these is GREEN.
var positionCalibrations = []calib{
	{1, true, 0, 0},
	{2, false, 20, 0},
	{3, false, 50, 0},
	{4, false, 100, 0},
	{5, false, 200, 0},
	{6, false, 300, 0},
	{7, false, 300, 100},
	{8, false, 200, 100},
	{9, false, 100, 100},
}

type touchCalib struct {
	picNum      int
	angle       float64
	separation  float64
	sameColours bool
}

// Primary barrel in all of these is GREEN.
var touchingCalibrations = []touchCalib{
	{10, 0, 0, false},
	{11, 90, 0, false},
	{12, 45, 0, false},
	{13, 0, 0, true},
	{14, 90, 0, true},
	{15, 45, 0, true},
	{16, 0, 20, true},
	{17, 90, 20, true},
	{18, 45, 20, true},
}
