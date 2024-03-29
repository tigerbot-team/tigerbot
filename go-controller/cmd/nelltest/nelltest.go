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
