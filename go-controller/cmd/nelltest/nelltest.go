package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/pkg/errors"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/ecodisaster"
)

type Context struct {
}

var CLI struct {
	Quit     QuitCmd     `cmd:"" help:"Quit"`
	Camera   CameraCmd   `cmd:"" help:"Command for camera subprocess"`
	Barrels  BarrelsCmd  `cmd:"" help:"Barrel route test"`
	Displace DisplaceCmd `cmd:"" help:""`
}

type QuitCmd struct{}

func (q *QuitCmd) Run(ctx *Context) error {
	return Quit
}

var Quit = errors.New("Quit")

type CameraCmd struct {
	Command string `arg:"" name:"command"`
}

func (c *CameraCmd) Run(ctx *Context) error {
	result, err := challengemode.CameraExecute(log.Printf, c.Command)
	log.Printf("CameraControl result=%v err=%v\n", result, err)
	return err
}

type BarrelsCmd struct {
	Command string `arg:"" name:"command"`
}

func (c *BarrelsCmd) Run(ctx *Context) error {
	ecodisaster.TestBarrels()
	return nil
}

type DisplaceCmd struct {
	Command string `arg:"" name:"command"`
}

func (c *DisplaceCmd) Run(ctx *Context) error {
	challengemode.TestDisplace2()
	return nil
}

func main() {
	fmt.Println("---- nelltest ----")
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0))

	k, err := kong.New(&CLI)
	if err != nil {
		panic(err)
	}

	ctx := &Context{}

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
