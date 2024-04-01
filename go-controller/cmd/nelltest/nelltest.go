package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/pkg/errors"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/cameracontrol"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder/angle"
)

var CLI struct {
	Quit QuitCmd `cmd:"" help:"Quit"`
	Sub  SubCmd  `cmd:"" help:"Command for subprocess."`
	Rm   struct {
		Force     bool `help:"Force removal."`
		Recursive bool `help:"Recursively remove files."`

		Paths []string `arg:"" name:"path" help:"Paths to remove." type:"path"`
	} `cmd:"" help:"Remove files."`

	Ls struct {
		Paths []string `arg:"" optional:"" name:"path" help:"Paths to list." type:"path"`
	} `cmd:"" help:"List paths."`
}

type Context struct {
	cameraControl *cameracontrol.CameraControl
}

type SubCmd struct {
	Command string `arg:"" name:"command"`
}

func (c *SubCmd) Run(ctx *Context) error {
	result, err := ctx.cameraControl.Execute(c.Command)
	log.Printf("CameraControl result=%v err=%v\n", result, err)
	return err
}

type QuitCmd struct{}

func (q *QuitCmd) Run(ctx *Context) error {
	return Quit
}

var Quit = errors.New("Quit")

func main() {
	fmt.Println("---- nelltest ----")
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0))

	k, err := kong.New(&CLI)
	if err != nil {
		panic(err)
	}

	ctx := &Context{
		cameraControl: cameracontrol.New(),
	}
	err = ctx.cameraControl.Start()
	if err != nil {
		panic(err)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Println("Enter a command:")
		if !scanner.Scan() {
			break
		}
		command := scanner.Text()
		parsed, err := k.Parse(strings.Split(command, " "))
		if err != nil {
			fmt.Println("parse error:", err)
			continue
		}
		err = parsed.Run(ctx)
		if err == Quit {
			break
		} else if err != nil {
			fmt.Println("ERROR:", err)
			continue
		}
	}
}

// Both of the following abstractions might include some uncertainties
// / error bars / values with probabilities attached.  But at any
// given time, when performing a challenge, we do need to decide what
// the bot should do next.  So we somehow need to keep those
// uncertainties small enough to at least take the next step.

// Abstraction of what we believe the state of the challenge arena to
// be, and how much of the challenge/arena remains to be done.
type Arena interface {
}

// Absolute HH heading that corresponds to the arena's positive X
// direction.
var xHeading float64

var hh hardware.HeadingAbsolute

const PositiveAnglesAnticlockwise float64 = 1

// Where we believe the bot to be within the arena, and its
// orientation at that position, w.r.t. a coordinate system that makes
// sense for the arena.
type ArenaPosition struct {
	x, y    float64 // millimetres
	heading float64 // w.r.t. the positive X direction, +tive CCW
}

type ThrottleRequest struct {
	angle    float64 // CCW from straight ahead (bot-relative)
	throttle float64
}

var lastThrottleRequest *ThrottleRequest

const RADIANS_PER_DEGREE = math.Pi / 180

func NewArenaPosition(old *ArenaPosition, rotations float64) *ArenaPosition {
	// Mapping from wheel rotations to actual ahead and sideways
	// displacement depends on the throttle angle.
	normalizedAngle := angle.FromFloat(lastThrottleRequest.angle).Float()

	// Round to the closest multiple of 5 degrees.  Note, Golang
	// rounds towards zero when converting float64 to int.
	var quantizedAngleOver5 int
	if normalizedAngle > 0 {
		quantizedAngleOver5 = int(normalizedAngle/5 + 0.5)
	} else {
		quantizedAngleOver5 = int(normalizedAngle/5 - 0.5)
	}

	// quantizedAngleOver5 is now from -36 to 36 (inclusive).
	aheadDisplacement := rotations * mmPerRotation[quantizedAngleOver5+36].ahead
	leftDisplacement := rotations * mmPerRotation[quantizedAngleOver5+36].left

	sin := math.Sin(old.heading * RADIANS_PER_DEGREE)
	cos := math.Cos(old.heading * RADIANS_PER_DEGREE)

	return &ArenaPosition{
		x:       old.x - aheadDisplacement*sin - leftDisplacement*cos,
		y:       old.y + aheadDisplacement*cos - leftDisplacement*sin,
		heading: old.heading,
	}
}

func Move(from, to *ArenaPosition) {
	if to.heading != from.heading {
		hh.SetThrottle(0)
		hh.SetHeading(xHeading + to.heading*PositiveAnglesAnticlockwise)
		hh.Wait(context.Background())
	}
	var displacementHeading float64
	if to.x == from.x {
		if to.y > from.y {
			displacementHeading = 90
		} else if to.y < from.y {
			displacementHeading = -90
		} else {
			return
		}
	} else if to.y == from.y {
		if to.x > from.x {
			displacementHeading = 0
		} else {
			displacementHeading = 180
		}
	} else {
		displacementHeading = math.Atan2(float64(to.y-from.y), float64(to.x-from.x)) / RADIANS_PER_DEGREE
	}
	newThrottleRequest := &ThrottleRequest{
		angle:    displacementHeading - to.heading,
		throttle: 1,
	}
	if lastThrottleRequest != nil && *newThrottleRequest != *lastThrottleRequest {
		hh.SetThrottle(newThrottleRequest)
		lastThrottleRequest = newThrottleRequest
	}
}

// Abstraction of some motion that we've already instructed the bot to
// perform, and that it has started and may still be performing.  (In
// principle could include either "do X until further notice" or "do X
// for the next T seconds".)
type Motion interface {
}

// Abstraction of the challenge as a whole.
type Challenge interface {
	StartingArena() Arena
	StartingPosition() Position
	AtEnd(Arena, Position) bool
	UpdateBeliefs(Arena, Position, time.Duration) (Arena, Position)
}

func logic() {

	var (
		arena     Arena
		position  Position
		challenge Challenge
	)

	challenge.Start()

	startTime := time.Now()

	for !challenge.AtEnd() {

		elapsedTime := time.Now().Sub(startTime)

		// Challenge iteration:
		// - Use sensors to update our beliefs about the arena
		//   and where we are within it.
		// - Compute the next position that we want the bot to
		//   move to.
		// - Compute how long to wait before calling Iterate
		//   again.
		nextTargetPosition, recheckTime = challenge.Iterate(elapsedTime)

		// Tell the bot to start (or continue) moving to that
		// position.
		bot.RequestMotionTo(nextTargetPosition)

		// Sleep for the indicated time before next iteration.
		time.Sleep(recheckTime)
	}
}
