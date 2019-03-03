package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcmode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcmode/duckshoot"

	"context"

	"github.com/fogleman/gg"

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

var (
	globalI2CLock sync.Mutex
	globalSPILock sync.Mutex
)

func main() {
	fmt.Println("---- Wall-E ----")
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0))

	// Our global context, we cancel it to trigger shutdown.
	ctx, cancel := context.WithCancel(context.Background())

	// Hook Ctrl-C etc.
	registerSignalHandlers(cancel)

	// Wait for the joystick and kick off a background thread to read from it.
	joystickEvents := initJoystick(cancel, ctx)

	// Initialise the hardware.
	hw := hardware.New()
	defer func() {
		fmt.Println("Zeroing motors for shut down")
		hw.Shutdown()
		time.Sleep(100 * time.Millisecond)
	}()
	hw.Start(ctx)

	hw.PlaySound("/sounds/tigerbotstart.wav")

	allModes := []Mode{
		rcmode.New("Duck shoot mode", "/sounds/duckshootmode.wav", hw, duckshoot.NewServoController()),
	}
	var activeMode Mode = allModes[0]
	fmt.Printf("----- %s -----\n", activeMode.Name())
	activeMode.Start(ctx)
	activeModeIdx := 0

	switchMode := func(delta int) {
		activeMode.Stop()
		hw.StopMotorControl()
		activeModeIdx += delta
		activeModeIdx = (activeModeIdx + len(allModes)) % len(allModes)
		activeMode = allModes[activeModeIdx]
		fmt.Printf("----- %s -----\n", activeMode.Name())

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
		fmt.Printf("Joy: %s\n", event)
		events <- event
	}
	return ctx.Err()
}

func drawTests() {
	f, err := os.OpenFile("/dev/fb1", os.O_RDWR, 0666)
	if err != nil {
		panic(err)
	}

	charge := 0.0
	for range time.NewTicker(500 * time.Millisecond).C {
		const S = 128
		dc := gg.NewContext(S, S)
		dc.SetRGBA(1, 0.9, 0, 1)
		//headingLock.Lock()
		//j := headingEstimate
		//headingLock.Unlock()
		//for i := 0; i < 360; i += 15 {
		//	dc.Push()
		//	dc.RotateAbout(gg.Radians(float64(i)+j), S/2, S/2)
		//	dc.DrawEllipse(S/2, S/2, S*7/16, S/8)
		//	dc.Fill()
		//	dc.Pop()
		//}

		dc.Push()
		dc.Translate(60, 5)
		dc.DrawString("CHARGE LVL", 0, 10)

		// Draw the larger power bar at the bottom. Colour depends on charge level.
		if charge < 0.1 {
			dc.SetRGBA(1, 0.2, 0, 1)
			dc.Push()
			dc.Translate(14, 80)
			DrawWarning(dc)
			dc.Pop()
		}

		dc.DrawRectangle(36, 70, 30, 10)

		for n := 2; n < 13; n++ {
			if charge >= (float64(n) / 13) {
				dc.DrawRectangle(38, 75-float64(n)*5, 26, 3)
			}
		}

		dc.Fill()

		dc.DrawString(fmt.Sprintf("%.1fv", 11.4+charge), 33, 93)

		dc.SetRGBA(1, 0.9, 0, 1)

		dc.Translate(14, 30)
		dc.Rotate(gg.Radians(1))
		dc.Scale(0.5, 1.0)
		dc.DrawRegularPolygon(3, 0, 0, 14, 0)
		dc.Fill()

		dc.Pop()

		charge += 0.1
		if charge > 1 {
			charge = 0
		}

		var buf [128 * 128 * 2]byte
		for y := 0; y < S; y++ {
			for x := 0; x < S; x++ {
				c := dc.Image().At(x, y)
				r, g, b, _ := c.RGBA() // 16-bit pre-multiplied

				rb := byte(r >> (16 - 5))
				gb := byte(g >> (16 - 6)) // Green has 6 bits
				bb := byte(b >> (16 - 5))

				buf[(127-y)*2+(x)*128*2+1] = (rb << 3) | (gb >> 3)
				buf[(127-y)*2+(x)*128*2] = bb | (gb << 5)
			}
		}
		_, err = f.Seek(0, 0)
		if err != nil {
			panic(err)
		}

		globalSPILock.Lock()
		_, err = f.Write(buf[:])
		globalSPILock.Unlock()
		if err != nil {
			panic(err)
		}
	}
}

func DrawWarning(dc *gg.Context) {
	dc.SetRGB(1, 0.2, 0)
	dc.DrawRegularPolygon(3, 0, 0, 14, 0)
	dc.Fill()
	dc.SetRGBA(0, 0, 0, 0.9)
	dc.DrawString("!", -3, 3)
}
