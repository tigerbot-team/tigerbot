package ecodisaster

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"
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

	// Use this state whenever we're just moving from one
	// known-safe position to another known-safe position.  In
	// this state we are not actively searching, picking up or
	// dropping off.  (We might already be carrying barrels
	// though, and/or moving in order to perform a drop-off.)  The
	// eventual target position is `moveTarget`, but we will get
	// there by moving through the safe areas around the edges of
	// the arena, and that will probably involve several
	// intermediate target positions.
	//
	// After arriving at `moveTarget`:
	//
	// - Transitions to DROP_OFF, if we need to drop off.
	//
	// - Transitions to SEARCH_OR_APPROACH if `decidedTarget` is
	// true.
	//
	// - Otherwise transitions to DECIDING_TARGET.
	MOVING_TO_SEARCH_POSITION

	// Deciding which barrel will be our target, or if we now need
	// to drop off.  May be colour-dependent, if the bot is
	// already carrying barrels.  (`botLoad` and `botColour`.)
	// `bestConfidence` holds the best confidence since we last
	// picked up (or lost target for) a barrel.
	// `bestSafePosition` is the position that we were at when we
	// got that confidence.  `decidedTarget` indicates when we
	// have decided what the next target will be.  `needDropOff`
	// indicates when we need to drop off.
	//
	// Transitions to MOVING_TO_SEARCH_POSITION to get to:
	//
	// - drop zone position, if drop-off need identified
	//
	// - `bestSafePosition`, if target has been chosen
	//
	// - next safe position to look from, otherwise.
	//
	// Transitions to APPROACHING_TARGET if sees an excellent
	// target from the current position.
	DECIDING_TARGET

	// Use this state to approach the chosen barrel target.  The
	// sequence of moves during this stage is stored in
	// `pathFromSafe`.
	//
	// Transitions to REVERSING_BACK_TO_SAFE after picking up the
	// barrel, or if we decide that we have lost the target.
	APPROACHING_TARGET

	// After a pick up, or after losing confidence in a target, we
	// reverse back to the safe position that we started from,
	// using `pathFromSafe`.
	//
	// `bestConfidence` is reset here.
	// `decidedTarget` is reset here.
	//
	// Transitions to DECIDING_TARGET after reaching the safe
	// position.
	REVERSING_BACK_TO_SAFE

	// Perform drop-off.
	//
	// Transitions to MOVING_TO_SEARCH_POSITION.  (Unless end of
	// challenge.)
	DROP_OFF
)

type challenge struct {
	hw hardware.Interface

	log   challengemode.Log
	stage stage

	// MOVING_TO_SEARCH_POSITION.
	moveTarget         challengemode.Position
	intermediateTarget challengemode.Position

	// APPROACHING_TARGET, REVERSING_BACK_TO_SAFE
	// Sequence of target positions we've moved through since the
	// last safe position.
	pathFromSafe []challengemode.Position

	// Number of barrels that the bot is already carrying, and the
	// colour of those barrels (all the same).
	botLoad   int
	botColour int

	// Searching.
	bestSafePosition challengemode.Position
	bestConfidence   float64
	decidedTarget    bool
	needDropOff      bool
}

func New(hw hardware.Interface) challengemode.Challenge {
	return &challenge{hw: hw}
}

func (c *challenge) Name() string {
	return "ECODISASTER"
}

func (c *challenge) Start(log challengemode.Log) (*challengemode.Position, bool) {
	c.log = log
	c.stage = MOVING_TO_SEARCH_POSITION
	c.log("Start")
	c.botLoad = 0
	c.decidedTarget = false
	c.needDropOff = false
	c.bestConfidence = 0

	// Assume we're initially positioned in the middle of the
	// start box.
	position := &challengemode.Position{
		X:              (xStartL + xStartR) / 2,
		Y:              (yStartB + yStartT) / 2,
		Heading:        90,
		HeadingIsExact: true,
	}

	// Target back to a safe position along the bottom wall.
	c.moveTarget = *position
	c.moveTarget.Y = dlBotWall
	c.intermediateTarget = c.moveTarget

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
	for {
		switch c.stage {
		case MOVING_TO_SEARCH_POSITION:
			if challengemode.TargetReached(&c.moveTarget, position) {
				c.log("Target (%v, %v, %v) reached", c.moveTarget.X, c.moveTarget.Y, c.moveTarget.Heading)
				if c.needDropOff {
					c.stage = DROP_OFF
				} else if c.decidedTarget {
					c.stage = APPROACHING_TARGET
				} else {
					c.stage = DECIDING_TARGET
				}
				goto stage_transition
			}
			if challengemode.TargetReached(&c.intermediateTarget, position) {
				c.log("Intermediate target (%v, %v, %v) reached", c.intermediateTarget.X, c.intermediateTarget.Y, c.intermediateTarget.Heading)
				c.calcNextIntermediateTarget(position)
			}
			return false, &c.intermediateTarget, time.Second

		case DECIDING_TARGET:
			if c.botLoad >= BOT_CAPACITY {
				c.needDropOff = true
				if c.botColour == RED {
					c.moveTarget.X = (xRedDropL + xRedDropR) / 2
					c.moveTarget.Y = ((2 * yDropB) + yDropT) / 3
					c.moveTarget.Heading = 180
				} else {
					c.moveTarget.X = (xGreenDropL + xGreenDropR) / 2
					c.moveTarget.Y = ((2 * yDropB) + yDropT) / 3
					c.moveTarget.Heading = 0
				}
				c.stage = MOVING_TO_SEARCH_POSITION
				goto stage_transition
			}

			// Try to identify a target in the direction
			// that we're currently facing - return a
			// confidence level for that, a heading
			// adjustment if we can see a barrel but it's
			// slightly to the left or right, and the
			// distance left to travel.
			c.log("searching for target")
			targetConfidence, headingAdjust, distance := c.IdentifyBarrel()
			c.log("targetConfidence %v headingAdjust %v distance %v", targetConfidence, headingAdjust, distance)
			if targetConfidence >= immediateConfidenceThreshold {
				c.bestConfidence = targetConfidence
				c.stage = APPROACHING_TARGET
				goto stage_transition
			}
			if targetConfidence > c.bestConfidence {
				c.bestConfidence = targetConfidence
				c.bestSafePosition = *position
				c.log("bestConfidence %v bestSafePosition %v", c.bestConfidence, c.bestSafePosition)
			}

			nextSafePosition := c.calcNextSafePosition(position)
			if nextSafePosition != nil {
				c.moveTarget = nextSafePosition
			} else {
				c.decidedTarget = true
				c.moveTarget = c.bestSafePosition
			}

			c.stage = MOVING_TO_SEARCH_POSITION
			goto stage_transition

		case APPROACHING_TARGET:
			c.log("approaching target")
			targetConfidence, headingAdjust, distance := c.IdentifyBarrel()
			c.log("targetConfidence %v headingAdjust %v distance %v", targetConfidence, headingAdjust, distance)

			// Check in case target confidence is going down.
			if targetConfidence < c.bestConfidence*allowedConfidenceDrop {
				c.log("Lost target")

				// Reverse out.
				c.stage = REVERSING_BACK_TO_SAFE
				goto stage_transition
			}

			if c.reachedTarget(headingAdjust, distance) {
				c.closeArms()
				c.botLoad++
				c.stage = REVERSING_BACK_TO_SAFE
				goto stage_transition
			}

			// Save current position, for reversing out later.
			c.pathFromSafe = append(c.pathFromSafe, *position)

			c.bestConfidence = targetConfidence
			target := &challengemode.Position{
				Heading: position.Heading + headingAdjust,
			}
			target.X = position.X + distance*math.Cos(target.Heading*challengemode.RADIANS_PER_DEGREE)
			target.Y = position.Y + distance*math.Sin(target.Heading*challengemode.RADIANS_PER_DEGREE)
			return false, target, time.Second

		case REVERSING_BACK_TO_SAFE:
			c.log("reversing back to safe")
			target := c.calcNextReverseTarget(position)
			if target == nil {
				c.stage = DECIDING_TARGET
				goto stage_transition
			}
			return false, target, time.Second

		case DROP_OFF:
			c.log("drop-off")
		}

	stage_transition:
		c.log("Stage => %v", c.stage)
		switch c.stage {
		case MOVING_TO_SEARCH_POSITION:
			c.calcNextIntermediateTarget(position)
		case APPROACHING_TARGET:
			c.pathFromSafe = nil
			c.openArms()
		case REVERSING_BACK_TO_SAFE:
			c.decidedTarget = false
			c.bestConfidence = 0
		}
	}
}

var waypoints = []challengemode.Position{
	{
		// Middle of top wall.
		X: dxTotal / 2,
		Y: dyTotal - dlBotWall,
	}, {
		// Top right corner.
		X: dxTotal - dlBotWall,
		Y: dyTotal - dlBotWall,
	}, {
		// Bottom right corner.
		X: dxTotal - dlBotWall,
		Y: dlBotWall,
	}, {
		// Bottom left corner.
		X: dlBotWall,
		Y: dlBotWall,
	}, {
		// Top left corner.
		X: dlBotWall,
		Y: dyTotal - dlBotWall,
	}, {
		// Middle of top wall.
		X: dxTotal / 2,
		Y: dyTotal - dlBotWall,
	},
}

var headingsDecreasingI = []float64{
	180,
	90,
	0,
	-90,
	180,
	0,
}

var headingsIncreasingI = []float64{
	0,
	0,
	-90,
	180,
	90,
	0,
}

func findWaypoint(position *challengemode.Position) int {
	leastDistance := float64(0)
	bestWaypoint := 0
	for i := 0; i < len(waypoints)-1; i++ {
		before := waypoints[i]
		after := waypoints[i+1]
		distance := float64(0)
		if before.X == after.X {
			distance = (position.X - before.X) * (position.X - before.X)
			mY := min(before.Y, after.Y)
			if position.Y < mY {
				distance += (position.Y - mY) * (position.Y - mY)
			}
			mY = max(before.Y, after.Y)
			if position.Y > mY {
				distance += (position.Y - mY) * (position.Y - mY)
			}
		} else {
			// Y coords are the same.
			distance = (position.Y - before.Y) * (position.Y - before.Y)
			mX := min(before.X, after.X)
			if position.X < mX {
				distance += (position.X - mX) * (position.X - mX)
			}
			mX = max(before.X, after.X)
			if position.X > mX {
				distance += (position.X - mX) * (position.X - mX)
			}
		}
		if i == 0 || distance < leastDistance {
			bestWaypoint = i
			leastDistance = distance
		}
	}
	return bestWaypoint
}
func (c *challenge) calcNextIntermediateTarget(position *challengemode.Position) {
	defer func() {
		c.log("intermediateTarget %v", c.intermediateTarget)
	}()

	if challengemode.TargetReachedIgnoreHeading(&c.moveTarget, position) {
		c.intermediateTarget = c.moveTarget
		return
	}

	ipos := findWaypoint(position)
	itarg := findWaypoint(&c.moveTarget)

reeval:
	if itarg < ipos {
		c.intermediateTarget = waypoints[ipos]
		c.intermediateTarget.Heading = headingsDecreasingI[ipos]
		if challengemode.TargetReachedIgnoreHeading(&c.intermediateTarget, position) {
			ipos -= 1
			goto reeval
		}
	} else if itarg > ipos {
		c.intermediateTarget = waypoints[ipos+1]
		c.intermediateTarget.Heading = headingsIncreasingI[ipos+1]
		if challengemode.TargetReachedIgnoreHeading(&c.intermediateTarget, position) {
			ipos += 1
			goto reeval
		}
	} else {
		c.intermediateTarget = c.moveTarget
		if waypoints[ipos].X == waypoints[ipos+1].X {
			if c.intermediateTarget.Y > position.Y {
				c.intermediateTarget.Heading = 90
			} else {
				c.intermediateTarget.Heading = -90
			}
		} else {
			if c.intermediateTarget.X > position.X {
				c.intermediateTarget.Heading = 0
			} else {
				c.intermediateTarget.Heading = 180
			}
		}
	}
}

func (c *challenge) calcNextReverseTarget(position *challengemode.Position) *challengemode.Position {
reeval:
	lenPFS := len(c.pathFromSafe)
	if lenPFS > 0 {
		c.log("moving back along recent path")
		lastTarget := c.pathFromSafe[lenPFS-1]
		c.log("lastTarget %v", lastTarget)
		if challengemode.TargetReached(&lastTarget, position) {
			c.log("reached last target")
			c.pathFromSafe = c.pathFromSafe[:lenPFS-1]
			goto reeval
		}
		if challengemode.TargetReachedIgnoreHeading(&lastTarget, position) {
			c.log("intermediate -> previous path point with its own heading")
			return &lastTarget
		}
		c.log("intermediate -> previous path point with current heading")
		lastTarget.Heading = position.Heading
		return &lastTarget
	}
	return nil
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

func (c *challenge) openArms() {
	c.hw.SetServo(0, -0.1)
	c.hw.SetServo(12, 1.4)
}

func (c *challenge) closeArms() {
	c.hw.SetServo(0, 1.4)
	c.hw.SetServo(12, 0.025)
}
