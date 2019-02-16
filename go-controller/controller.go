package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/fogleman/gg"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hw"
	imu2 "github.com/tigerbot-team/tigerbot/go-controller/pkg/imu"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"

	"context"

	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/wav"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mazemode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/pausemode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rainbowmode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcmode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rcmode/duckshoot"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/slstmode"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/testmode"
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
	fmt.Print("---- Tigerbot ----\n\n")
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0))

	// Our global context, we cancel it to trigger shutdown.
	ctx, cancel := context.WithCancel(context.Background())

	// Hook Ctrl-C etc.
	registerSignalHandlers(cancel)

	// Wait for the joystick and kick off a background thread to read from it.
	joystickEvents := initJoystick(cancel, ctx)

	// Initialise the SPI hardware.

	// Screen.
	go drawTests()

	// IMU.
	imu, err := imu2.NewSPI("/dev/spidev0.1")
	if err != nil {
		fmt.Println("Failed to open IMU", err)
		panic("Failed to open IMU")
	}
	err = imu.Configure()
	if err != nil {
		fmt.Println("Failed to configure IMU", err)
		panic("Failed to open IMU")
	}
	time.Sleep(1 * time.Second)
	err = imu.Calibrate()
	if err != nil {
		fmt.Println("Failed to calibrate IMU", err)
		panic("Failed to open IMU")
	}
	imu.ResetFIFO()

	// Initialise the I2C hardware.  First the mux, which all the other peripherals rely on.
	globalI2CLock.Lock()
	i2cMux, err := mux.New("/dev/i2c-1")
	globalI2CLock.Unlock()
	if err != nil {
		fmt.Println("Failed to open mux", err)
		return
	}
	// TODO need to keep reading from the FIFO.

	// Voltage monitors, background thread.
	// Time of flight sensors.
	// Servo controller.

	// Prooeller
	globalI2CLock.Lock()
	prop, err := propeller.New(i2cMux, 7)
	globalI2CLock.Unlock()
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
	fmt.Println("Zeroing motors for startup")
	globalI2CLock.Lock()
	err = prop.SetMotorSpeeds(0, 0)
	globalI2CLock.Unlock()
	if err != nil {
		panic(err)
	}
	defer func() {
		fmt.Println("Zeroing motors for shut down")
		globalI2CLock.Lock()
		defer globalI2CLock.Unlock()
		_ = prop.SetMotorSpeeds(0, 0)
	}()

	// Init the sound system and play the startup sound.
	soundsToPlay := initSounds()
	defer close(soundsToPlay)
	soundsToPlay <- "/sounds/tigerbotstart.wav"

	hw := &hw.Hardware{
		Motors:          prop,
		DistanceSensors: nil,
		I2CLock:         &globalI2CLock,
		SPILock:         &globalSPILock,
		IMU:             imu,
		Mux:             i2cMux,
		ServoController: duckshoot.NewServoController(),
	}

	allModes := []Mode{
		rcmode.New("Golf mode", "/sounds/tigerbotstart.wav", hw),
		rcmode.New("Duck shoot mode", "/sounds/duckshootmode.wav", hw),
		mazemode.New(hw, soundsToPlay),
		slstmode.New(hw, soundsToPlay),
		rainbowmode.New(hw, soundsToPlay),
		&testmode.TestMode{Propeller: hw.Motors},
		&pausemode.PauseMode{Propeller: hw.Motors},
	}
	var activeMode Mode = allModes[0]
	fmt.Printf("----- %s -----\n", activeMode.Name())
	activeMode.Start(ctx)
	activeModeIdx := 0

	switchMode := func(delta int) {
		activeMode.Stop()
		err = prop.SetMotorSpeeds(0, 0)
		if err != nil {
			panic(err)
		}
		activeModeIdx += delta
		activeModeIdx = (activeModeIdx + len(allModes)) % len(allModes)
		activeMode = allModes[activeModeIdx]
		fmt.Printf("----- %s -----\n", activeMode.Name())

		soundsToPlay <- activeMode.StartupSound()

		activeMode.Start(ctx)
	}

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
				ju.OnJoystickEvent(event)
			}
		}
	}
}

func initSounds() chan string {
	soundsToPlay := make(chan string)
	go func() {
		defer func() {
			recover()
			for s := range soundsToPlay {
				fmt.Println("Unable to play", s)
			}
		}()
		sampleRate := beep.SampleRate(44100)
		err := speaker.Init(sampleRate, sampleRate.N(time.Second/5))
		if err != nil {
			fmt.Println("Failed to open speaker", err)
			for s := range soundsToPlay {
				fmt.Println("Unable to play", s)
			}
		}
		var ctrl *beep.Ctrl
		var s beep.StreamSeekCloser
		for soundToPlay := range soundsToPlay {
			if ctrl != nil {
				speaker.Lock()
				ctrl.Paused = true
				ctrl.Streamer = nil
				speaker.Unlock()
				ctrl = nil
			}
			if s != nil {
				s.Close()
			}

			f, err := os.Open(soundToPlay)
			if err != nil {
				fmt.Println("Failed to open sound", err)
				continue
			}
			s, _, err = wav.Decode(f)
			if err != nil {
				fmt.Println("Failed to decode sound", err)
				continue
			}
			ctrl := &beep.Ctrl{Streamer: s}
			speaker.Play(ctrl)
		}
	}()
	return soundsToPlay
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
