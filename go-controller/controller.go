package main

import (
	"fmt"
	"time"

	"context"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/pausemode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcmode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/testmode"
)

type Mode interface {
	Name() string
	Start(ctx context.Context)
	Stop()
}

type JoystickUser interface {
	OnJoystickEvent(event *joystick.Event)
}

func main() {
	fmt.Print("---- Tigerbot ----\n\n")

	// Our global context, we cancel it to trigger shutdown.
	ctx, cancel := context.WithCancel(context.Background())

	joystickEvents := make(chan *joystick.Event)
	for {
		j, err := joystick.NewJoystick("/dev/input/js0")
		if err != nil {
			fmt.Printf("Failed to open joystick: %v.\n", err)
			time.Sleep(1 * time.Second)
			continue
		}

		fmt.Printf("Opened joystick\n")
		go func() {
			defer cancel()
			defer j.Close()
			err := loopReadingJoystickEvents(ctx, j, joystickEvents)
			fmt.Printf("Joystick failed: %v\n", err)
		}()
		break
	}

	p, err := propeller.New()
	if err != nil {
		fmt.Printf("Failed to open propeller: %v.\n", err)
		cancel()
		return
	}

	fmt.Println("Zeroing motors")
	err = p.SetMotorSpeeds(0, 0, 0, 0)
	if err != nil {
		panic(err)
	}
	defer func() {
		fmt.Println("Zeroing motors")
		p.SetMotorSpeeds(0, 0, 0, 0)
	}()

	allModes := []Mode{
		&pausemode.PauseMode{Propeller: p},
		rcmode.New(p),
		&testmode.TestMode{Propeller: p},
	}
	var activeMode Mode = allModes[0]
	fmt.Printf("----- %s -----\n", activeMode.Name())
	activeMode.Start(ctx)
	activeModeIdx := 0

	for {
		select {
		case event, ok := <-joystickEvents:
			if !ok {
				fmt.Println("Joystick events channel closed!")
				cancel()
				time.Sleep(1 * time.Second)
				return
			}
			// Intercept the Options button to implement mode switching.
			if event.Type == joystick.EventTypeButton &&
				event.Number == joystick.ButtonOptions &&
				event.Value == 1 {
				fmt.Printf("Options pressed: switching modes.\n")
				activeMode.Stop()
				err = p.SetMotorSpeeds(0, 0, 0, 0)
				if err != nil {
					panic(err)
				}
				activeModeIdx++
				activeModeIdx = activeModeIdx % len(allModes)
				activeMode = allModes[activeModeIdx]
				fmt.Printf("----- %s -----\n", activeMode.Name())
				activeMode.Start(ctx)
				continue
			}
			// Pass other joystick events through if this mode requires them.
			if ju, ok := activeMode.(JoystickUser); ok {
				ju.OnJoystickEvent(event)
			}
		}
	}
}

func loopReadingJoystickEvents(ctx context.Context, j *joystick.Joystick, events chan *joystick.Event) error {
	defer close(events)
	defer j.Close()
	for ctx.Err() == nil {
		event, err := j.ReadEvent()
		if err != nil {
			fmt.Printf("Failed to read from joystick: %v.\n", err)
			return err
		}
		fmt.Printf("Event from joystick: %s\n", event)
		events <- event
	}
	return ctx.Err()
}
