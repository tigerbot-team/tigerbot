package main

import (
	"fmt"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mazemode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/slstmode"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/screen"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/testmode"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/nebulamode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/pausemode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcmode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcmode/duckshoot"

	"context"

	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
)

type Mode interface {
	Name() string
	StartupSound() string
	Start(ctx context.Context)
	Stop()
}

type JoystickUser interface {
	OnJoystickEvent(event *joystick.Event)
}

func main() {
	fmt.Println("---- Wall-E ----")
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0))

	// Our global context, we cancel it to trigger shutdown.
	ctx, cancel := context.WithCancel(context.Background())

	// Hook Ctrl-C etc.
	registerSignalHandlers(cancel)

	// Initialise the hardware.
	hw := hardware.New()
	defer func() {
		fmt.Println("Zeroing motors for shut down")
		hw.Shutdown()
		time.Sleep(100 * time.Millisecond)
	}()
	hw.Start(ctx)

	// Wait for the joystick and kick off a background thread to read from it.
	joystickEvents := initJoystick(cancel, ctx)

	hw.PlaySound("/sounds/tigerbotstart.wav")

	allModes := []Mode{
		rcmode.New("GUN MODE", "/sounds/duckshootmode.wav", hw, duckshoot.NewServoController()),
		mazemode.New(hw),
		slstmode.New(hw),
		pausemode.New(hw),
		testmode.New(hw),
		nebulamode.New(hw),
	}
	var activeMode Mode = allModes[0]
	fmt.Printf("----- %s -----\n", activeMode.Name())
	screen.SetMode(activeMode.Name())
	activeMode.Start(ctx)
	activeModeIdx := 0

	switchMode := func(delta int) {
		fmt.Println("Mode switch", delta)
		activeMode.Stop()
		fmt.Println("Mode switch: active mode stopped", delta)
		hw.StopMotorControl()
		fmt.Println("Mode switch: motors stopped", delta)
		activeModeIdx += delta
		activeModeIdx = (activeModeIdx + len(allModes)) % len(allModes)
		activeMode = allModes[activeModeIdx]
		fmt.Printf("----- %s -----\n", activeMode.Name())
		screen.SetMode(activeMode.Name())

		hw.PlaySound(activeMode.StartupSound())

		activeMode.Start(ctx)
		fmt.Println("Mode switch done.")
	}

	fmt.Println("Waiting for events...", joystickEvents)
	watchdog := time.NewTicker(5 * time.Second)

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
				fmt.Printf("Options pressed: switching modes >>\n")
				switchMode(1)
				continue
			} else if event.Type == joystick.EventTypeButton &&
				event.Number == joystick.ButtonShare &&
				event.Value == 1 {
				fmt.Printf("Share pressed: switching modes <<\n")
				switchMode(-1)
				continue
			}
			// Pass other joystick events through if this mode requires them.
			if ju, ok := activeMode.(JoystickUser); ok {
				done := make(chan struct{})
				go func() {
					defer close(done)
					ju.OnJoystickEvent(event)
				}()
				timeout := time.NewTimer(1 * time.Second)
				select {
				case <-done:
					timeout.Stop()
				case <-timeout.C:
					// All the modes are supposed to just queue the event to the background thread.
					// If they block this long, they've probably deadlocked.
					panic("Deadlock? Active mode blocked OnJoystickEvent for >1s")
				}
			}
		case <-watchdog.C:
			fmt.Println("Main loop still running")
		}
	}
}

func initJoystick(cancel context.CancelFunc, ctx context.Context) chan *joystick.Event {
	joystickEvents := make(chan *joystick.Event, 1)
	firstLog := true
	for {
		jDev := os.Getenv("JOYSTICK_DEVICE")
		if jDev == "" {
			jDev = "/dev/input/js0"
		}
		j, err := joystick.NewJoystick(jDev)
		const noJoy = "NO JOY"
		if err != nil {
			if firstLog {
				screen.SetNotice(noJoy, screen.LevelErr)
				fmt.Printf("Waiting for joystick: %v.\n", err)
				firstLog = false
			}
			time.Sleep(1 * time.Second)
			continue
		}

		screen.ClearNotice(noJoy)
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
		fmt.Printf("Joy: %s\n", event)
		events <- event
	}
	return ctx.Err()
}
