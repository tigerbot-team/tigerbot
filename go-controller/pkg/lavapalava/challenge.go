package lavapalava

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

const (
	dxWidth  = float64(550)
	dyLength = float64(7000)
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
		Heading: 90,
		X:       dxWidth / 2,
		Y:       0,
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
	targetAhead, targetLeft, headingAdjust := c.AnalyseWhiteLine()
	dx, dy := challengemode.AbsoluteDeltas(position.Heading, targetAhead, targetLeft)

	target := &challengemode.Position{
		Heading: position.Heading + headingAdjust,
		X:       position.X + dx,
		Y:       position.Y + dy,
	}

	return false, target, 500 * time.Millisecond
}

func (c *challenge) AnalyseWhiteLine() (float64, float64, float64) {
	rsp, err := challengemode.CameraExecute(c.log, "white-line")
	if err != nil {
		c.log("AnalyseWhiteLine camera err=%v", err)
	}
	rspWords := strings.Split(rsp, " ")
	gradient, err := strconv.ParseFloat(rspWords[0], 64)
	if err != nil {
		c.log("gradient '%v' err=%v", rspWords[0], err)
	}
	centre, err := strconv.ParseFloat(rspWords[1], 64)
	if err != nil {
		c.log("centre '%v' err=%v", rspWords[1], err)
	}

	// `gradient` indicates how much the line is moving to the
	// right (+tive) or left (-tive).
	//
	// `centre` indicates if the closest part of the line is in
	// front of the centre of the bot (100), or to the left of
	// bot's centre (< 100) or to the right of bot's centre (>
	// 100).
	//
	// We want to align the bot's heading with the gradient and
	// target a position at the far end of the gradient line.  The
	// tricky part here is just how we map from picture
	// coordinates to X, Y on the ground.
	//
	// From other calibration work: half way up the photo
	// corresponds to about 28cm ahead of the bot; photo width at
	// that distance corresponds to 72cm; and photo width at the
	// bottom of the photo corresponds to 37cm.
	targetAhead := float64(280)
	targetLeft := (centre + 19*gradient - 100) * 720.0 / 200.0
	closeLeft := (centre - 100) * 370.0 / 200.0
	headingAdjust := math.Atan2(targetLeft-closeLeft, targetAhead)

	return targetAhead, targetLeft, headingAdjust
}
