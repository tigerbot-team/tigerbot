package zombie

import (
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

const (
	// Total arena size.
	dxTotal = float64(1150)
	dyTotal = float64(900)

	startX  = float64(200)
	startY  = float64(200)
	targetY = float64(600)

	fixedHeading = 90
)

type stage int

const (
	INIT stage = iota
	HUNT_RIGHT
	HUNT_LEFT
)

type challenge struct {
	log           challengemode.Log
	stage         stage
	xTarget       float64
	yTarget       float64
	headingTarget float64
	hw            hardware.Interface
}

func New(hw hardware.Interface) challengemode.Challenge {
	return &challenge{hw: hw}
}

func (c *challenge) Name() string {
	return "ZOMBIE"
}

func (c *challenge) Start(log challengemode.Log) (*challengemode.Position, bool) {
	c.log = log
	c.stage = INIT

	// Assume we're initially positioned facing straight down the course to the zombies
	// at the left of the arena.
	position := &challengemode.Position{
		X:              200,
		Y:              200,
		Heading:        fixedHeading,
		HeadingIsExact: true,
	}

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
	switch c.stage {
	case INIT:
		initialTarget := &challengemode.Position{
			X:       startX,
			Y:       targetY,
			Heading: fixedHeading,
		}

		if challengemode.TargetReached(initialTarget, position) {
			c.stage = HUNT_RIGHT
			return false, initialTarget, 0
		}

		return false, initialTarget, 1 * time.Second
	case HUNT_RIGHT:
		target := &challengemode.Position{
			X:       dxTotal - 200,
			Y:       targetY,
			Heading: fixedHeading,
		}

		if challengemode.TargetReached(target, position) {
			c.stage = HUNT_LEFT
			return false, target, 0
		}

		challengemode.CameraExecute(c.log, "find-zombies")

		return false, target, 1 * time.Second
	case HUNT_LEFT:
		target := &challengemode.Position{
			X:       250,
			Y:       targetY,
			Heading: fixedHeading,
		}

		if challengemode.TargetReached(target, position) {
			c.stage = HUNT_RIGHT
			return false, target, 0
		}

		challengemode.CameraExecute(c.log, "find-zombies")

		return false, target, 1 * time.Second
	}
	panic("unknown stage")
}

var motorInitOnce sync.Once

const (
	motor1     = 14
	motor2     = 15
	reload     = 13
	reloadfwd  = 0.0
	reloadback = 1.0
	v          = 0.5
)

func (c *challenge) Fire() {
	motorInitOnce.Do(func() {
		fmt.Println("Doing motor one-off init.")
		c.hw.SetServo(motor1, 0)
		c.hw.SetServo(motor2, 0)
		time.Sleep(5 * time.Second)
	})

	fmt.Printf("Spin up...\n")
	c.hw.SetServo(reload, reloadback)
	for i := 0; i < 100; i++ {
		c.hw.SetServo(motor1, v*float64(i)/100)
		time.Sleep(10 * time.Millisecond)
		c.hw.SetServo(motor2, v*float64(i)/100)
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(5000 * time.Millisecond)
	fmt.Printf("Firing...\n")
	c.hw.SetServo(reload, reloadfwd)
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("Resetting\n")
	c.hw.SetServo(reload, reloadback)
	c.hw.SetServo(motor1, 0)
	c.hw.SetServo(motor2, 0)
}

func (c *challenge) SpeedMMPerS() float64 {
	return 100
}
