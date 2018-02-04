package main

import (
	"fmt"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"time"
)

func main()  {
	fmt.Print("---- Tigerbot ----\n\n")

	restartLoop: for {
		j, err := joystick.NewJoystick("/dev/input/js0")
		if err != nil {
			fmt.Printf("Failed to open joystick: %v.\n", err)
			time.Sleep(1 * time.Second)
			continue
		}
		for {
			event, err := j.ReadEvent()
			if err != nil {
				j.Close()
				fmt.Printf("Failed to read from joystick: %v.\n", err)
				time.Sleep(1 * time.Second)
				continue restartLoop
			}
			fmt.Printf("Event from joystick: %s\n", event)
		}
	}
}
