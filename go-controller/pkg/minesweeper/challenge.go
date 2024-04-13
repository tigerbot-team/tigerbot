package minesweeper

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

const (
	// Total arena size.
	dxTotal = float64(1600)
	dyTotal = float64(1600)

	// Size of the starting box.
	dyInitial = float64(400)
	dxInitial = float64(400)

	// Confidence level at which we'll immediately start heading
	// to the next square, even if we haven't done a full rotation
	// of searching yet.
	immediateConfidenceThreshold = float64(90)

	// Measure of how much the confidence level could drop by once
	// we're already approaching the believed target.  In other
	// words, if the current confidence is less that this factor
	// times the confidence when we identified the target, we'll
	// decide we made a mistake and restart the search.
	allowedConfidenceDrop = float64(0.8)
)

type stage int

const (
	INIT stage = iota
	POSSIBLY_UNSAFE_FOR_SEARCH
	SAFE_FOR_SEARCH
	ON_BOMB_SQUARE
)

type challenge struct {
	log               challengemode.Log
	stage             stage
	xTarget           float64
	yTarget           float64
	headingTarget     float64
	obeyHeadingTarget bool

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
	return "MINESWEEPER"
}

func (c *challenge) Start(log challengemode.Log) (*challengemode.Position, bool) {
	c.log = log
	c.stage = INIT

	// Assume we're initially positioned in the middle of the
	// bottom left corner square.  Don't know the heading, but
	// calibration should tell us that.
	position := &challengemode.Position{
		X: dxInitial / 2,
		Y: dyInitial / 2,
	}

	// Target the same position, so we'll immediately transition
	// to the next stage.
	c.xTarget = position.X
	c.yTarget = position.Y

	return position, true
}

func (c *challenge) IdentifyMine() (confidence, headingAdjust, distance float64) {
	rsp, err := challengemode.CameraExecute("id-mine")
	if err != nil {
		c.log("IdentifyMine camera err=%v", err)
	}
	rspWords := strings.Split(rsp, " ")
	confidence, err = strconv.ParseFloat(rspWords[0], 64)
	if err != nil {
		c.log("confidence '%v' err=%v", rspWords[0], err)
	}
	headingAdjust, err = strconv.ParseFloat(rspWords[1], 64)
	if err != nil {
		c.log("headingAdjust '%v' err=%v", rspWords[1], err)
	}
	distance, err = strconv.ParseFloat(rspWords[2], 64)
	if err != nil {
		c.log("distance '%v' err=%v", rspWords[2], err)
	}
	return
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
		case SAFE_FOR_SEARCH:
			// Try to identify a target in the direction
			// that we're currently facing - return a
			// confidence level for that, a heading
			// adjustment if we can see a red square but
			// it's slightly to the left or right, and the
			// distance left to travel in order for part
			// of the bot to be over the square.
			targetConfidence, headingAdjust, distance := c.IdentifyMine()
			if c.approachingTarget {
				// Check in case target confidence is going down.
				if targetConfidence < c.bestConfidence*allowedConfidenceDrop {
					c.log("Lost target")

					// Restart the search.
					c.stage = POSSIBLY_UNSAFE_FOR_SEARCH
					return false, position, 0
				}
				c.bestHeading = position.Heading + headingAdjust
			} else {
				if targetConfidence > c.bestConfidence {
					c.bestConfidence = targetConfidence
					c.bestHeading = position.Heading + headingAdjust
				}
				nextHeading := position.Heading + 40
				if targetConfidence < immediateConfidenceThreshold &&
					nextHeading < c.searchInitialHeading+360 {
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
				c.approachingTarget = true
			}

			c.xTarget = position.X + distance*math.Cos(c.bestHeading*challengemode.RADIANS_PER_DEGREE)
			c.yTarget = position.Y + distance*math.Sin(c.bestHeading*challengemode.RADIANS_PER_DEGREE)
			c.headingTarget = c.bestHeading
			c.obeyHeadingTarget = true
		}

		// Return the current target, if we haven't yet
		// reached it.
		target := &challengemode.Position{
			X:       c.xTarget,
			Y:       c.yTarget,
			Heading: position.Heading,
		}
		if c.obeyHeadingTarget {
			target.Heading = c.headingTarget
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
		case POSSIBLY_UNSAFE_FOR_SEARCH:
			// Move to a position at least one square side
			// away from the walls.
			c.xTarget = min(max(position.X, dxInitial), dxTotal-dxInitial)
			c.yTarget = min(max(position.Y, dyInitial), dyTotal-dyInitial)
			c.obeyHeadingTarget = false
		case SAFE_FOR_SEARCH:
			// Beginning a search; store the initial
			// heading, so we don't rotate forever.
			c.searchInitialHeading = position.Heading
			c.bestConfidence = 0
		case ON_BOMB_SQUARE:
			// Sit here for 1 second.
			time.Sleep(1 * time.Second)

			// Restart the search.
			c.stage = POSSIBLY_UNSAFE_FOR_SEARCH
			return false, position, 0
		}
	}
}
