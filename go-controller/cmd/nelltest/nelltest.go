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
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/picobldc"
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

const PositiveAnglesAnticlockwise float64 = 1

// Where we believe the bot to be within the arena, and its
// orientation at that position, w.r.t. a coordinate system that makes
// sense for the arena.
type Position struct {
	x, y    float64 // millimetres
	heading float64 // w.r.t. the positive X direction, +tive CCW
}

type ThrottleRequest struct {
	angle    float64 // CCW from straight ahead (bot-relative)
	throttle float64
}

var lastThrottleRequest *ThrottleRequest

const RADIANS_PER_DEGREE = math.Pi / 180

func MakeUpdatePosition(lastRotations picobldc.PerMotorVal[float64]) func(position *Position, newRotations picobldc.PerMotorVal[float64]) {
	return func(position *Position, newRotations picobldc.PerMotorVal[float64]) {
		// Calculate an overall "rotations" number that we
		// will use to scale our calibration table.
		rotations := float64(0)
		for m := range newRotations {
			rotations += math.Abs(newRotations[m] - lastRotations[m])
			lastRotations[m] = newRotations[m]
		}

		// Mapping from wheel rotations to actual ahead and
		// sideways displacement depends on the throttle
		// angle.
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

		sin := math.Sin(position.heading * RADIANS_PER_DEGREE)
		cos := math.Cos(position.heading * RADIANS_PER_DEGREE)

		position.x -= (aheadDisplacement*sin + leftDisplacement*cos)
		position.y += (aheadDisplacement*cos - leftDisplacement*sin)
	}
}

func StartMotion(hh hardware.HeadingAbsolute, current, target *Position) {
	if target.heading != current.heading {
		hh.SetHeading(xHeading + target.heading*PositiveAnglesAnticlockwise)
		hh.Wait(context.Background())
	}
	var displacementHeading float64
	if target.x == current.x {
		if target.y > current.y {
			displacementHeading = 90
		} else if target.y < current.y {
			displacementHeading = -90
		} else {
			return
		}
	} else if target.y == current.y {
		if target.x > current.x {
			displacementHeading = 0
		} else {
			displacementHeading = 180
		}
	} else {
		displacementHeading = math.Atan2(float64(target.y-current.y), float64(target.x-current.x)) / RADIANS_PER_DEGREE
	}
	newThrottleRequest := &ThrottleRequest{
		angle:    displacementHeading - target.heading,
		throttle: 1,
	}
	if lastThrottleRequest != nil && *newThrottleRequest != *lastThrottleRequest {
		hh.SetThrottle(newThrottleRequest)
		lastThrottleRequest = newThrottleRequest
	}
}

func TargetReached(currentTarget, position *Position) bool {
	const maxDelta float64 = 10 // millimetres
	return (math.Abs(currentTarget.x-position.x) <= maxDelta &&
		math.Abs(currentTarget.y-position.y) <= maxDelta)
}

type Challenge interface {
	// Set any internal state to reflect the beginning of the
	// challenge and return the initial bot position.
	Start() *Position

	// Use available sensors to update our beliefs about the arena
	// and where we are within it.
	Iterate(position, target *Position, timeSinceStart time.Duration) (bool, *Position, time.Duration)
}

func doChallenge(challenge Challenge, h hardware.Interface) {

	hh := h.StartHeadingHoldMode()
	position := challenge.Start()
	startTime := time.Now()
	target := (*Position)(nil)
	UpdatePosition := MakeUpdatePosition(h.AccumulatedRotations())

	for {
		// Note, the bot is stationary at the start of each
		// iteration of this loop.

		timeSinceStart := time.Now().Sub(startTime)

		// Challenge-specific iteration: given current
		// position, current target, and time since start of
		// challenge,
		//
		// - Use sensors to update where we think we are and
		//   what we think the arena is.
		//
		// - Decide if we've reached the end of the challenge.
		//   If not...
		//
		// - Compute the next target position that the bot
		//   should move towards, and for how long it should
		//   do that before re-evaluating.
		atEnd, target, moveTime := challenge.Iterate(position, target, timeSinceStart)
		if atEnd {
			break
		}

		// Start moving to the target position.
		StartMotion(hh, position, target)

		// Allow motion for the indicated time.
		time.Sleep(moveTime)

		// Stop moving.
		hh.SetThrottle(0)

		// Update current position based on dead reckoning.
		UpdatePosition(position, h.AccumulatedRotations())
	}
}
