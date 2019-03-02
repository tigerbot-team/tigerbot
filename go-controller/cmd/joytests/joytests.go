package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
)

func main() {
	// Our global context, we cancel it to trigger shutdown.
	ctx, cancel := context.WithCancel(context.Background())

	// Hook Ctrl-C etc.
	registerSignalHandlers(cancel)

	// Wait for the joystick and kick off a background thread to read from it.
	joystickEvents := initJoystick(cancel, ctx)
	for je := range joystickEvents {
		fmt.Println(je)
	}
}

func initJoystick(cancel context.CancelFunc, ctx context.Context) chan *joystick.Event {
	joystickEvents := make(chan *joystick.Event)
	firstLog := true
	for {
		jDev := os.Getenv("JOYSTICK_DEVICE")
		if jDev == "" {
			jDev = "/dev/input/js0"
		}
		j, err := joystick.NewJoystick(jDev)
		if err != nil {
			if firstLog {
				fmt.Printf("Waiting for joystick: %v.\n", err)
				firstLog = false
			}
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
	return joystickEvents
}

func registerSignalHandlers(cancelFunc context.CancelFunc) {
	// Hook Ctrl-C to cause shut down.
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-signals
		log.Println("Signal: ", s)
		cancelFunc()
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()
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
