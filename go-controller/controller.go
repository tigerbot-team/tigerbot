package main

import (
	"fmt"
	"time"

	"context"

	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mazemode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/pausemode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rainbowmode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcmode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/slstmode"
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
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0))

	// Our global context, we cancel it to trigger shutdown.
	ctx, cancel := context.WithCancel(context.Background())

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		s := <-signals
		log.Println("Signal: ", s)
		cancel()
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()

	joystickEvents := make(chan *joystick.Event)
	for {
		jDev := os.Getenv("JOYSTICK_DEVICE")
		if jDev == "" {
			jDev = "/dev/input/js0"
		}
		j, err := joystick.NewJoystick(jDev)
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

	prop, err := propeller.New()
	if err != nil {
		fmt.Printf("Failed to open propeller: %v.\n", err)
		if os.Getenv("IGNORE_MISSING_PROPELLER") == "true" {
			fmt.Printf("Using dummy propeller\n")
			prop = propeller.Dummy()
		} else {
			cancel()
			return
		}
	}

	fmt.Println("Zeroing motors")
	err = prop.SetMotorSpeeds(0, 0, 0, 0)
	if err != nil {
		panic(err)
	}
	defer func() {
		fmt.Println("Zeroing motors")
		prop.SetMotorSpeeds(0, 0, 0, 0)
	}()

	allModes := []Mode{
		rcmode.New(prop),
		mazemode.New(prop),
		slstmode.New(prop),
		rainbowmode.New(prop),
		&testmode.TestMode{Propeller: prop},
		&pausemode.PauseMode{Propeller: prop},
	}
	var activeMode Mode = allModes[0]
	fmt.Printf("----- %s -----\n", activeMode.Name())
	activeMode.Start(ctx)
	activeModeIdx := 0

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Context done, stopping active mode and shutting down")
			activeMode.Stop()
			cancel()
			time.Sleep(1 * time.Second)
			return
		case event, ok := <-joystickEvents:
			if !ok {
				fmt.Println("Joystick events channel closed!")
				activeMode.Stop()
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
				err = prop.SetMotorSpeeds(0, 0, 0, 0)
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
