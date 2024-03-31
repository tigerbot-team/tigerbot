package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/pkg/errors"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/cameracontrol"
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

// Abstraction of where we believe the bot to be within the arena, and
// its orientation at that position.
type Position interface {
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
