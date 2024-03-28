package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/picobldc"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
)

func main() {
	// Our global context, we cancel it to trigger shutdown.
	ctx, cancel := context.WithCancel(context.Background())

	// Hook Ctrl-C etc.
	registerSignalHandlers(cancel)

	// Wait for the joystick and kick off a background thread to read from it.
	joystickEvents := initJoystick(cancel, ctx)
	ticker := time.NewTicker(10 * time.Millisecond)
	metricsTicker := time.NewTicker(time.Second)

	pico, err := picobldc.New()
	if err != nil {
		panic(err)
	}
	fmt.Println("Created PicoBLDC object. Enabling watchdog...")

	if err := pico.SetWatchdog(time.Second); err != nil {
		panic(err)
	}
	fmt.Println("Watchdog enabled.")

	var throttle, translation, rotation float64

	for {
		select {
		case je := <-joystickEvents:
			switch je.Type {
			case joystick.EventTypeAxis:
				switch je.Number {
				case joystick.AxisLStickY:
					throttle = applyExpo(float64(je.Value)/(-math.MaxInt16), 1.6)
				case joystick.AxisLStickX:
					translation = applyExpo(float64(je.Value)/(-math.MaxInt16), 1.6)
				case joystick.AxisRStickX:
					rotation = applyExpo(float64(je.Value)/(-math.MaxInt16), 1.6)
				}
				continue
			case joystick.EventTypeButton:
				continue
			}
		case <-ticker.C:

			// Map the values to speeds for each motor.  Motor rotation direction:
			// positive = anti-clockwise.
			frontLeft := throttle - rotation - translation
			backLeft := throttle - rotation + translation

			frontRight := -throttle - rotation - translation
			backRight := -throttle - rotation + translation

			m := max(frontLeft, frontRight, backLeft, backRight)
			scale := 1.0
			if m > 1 {
				scale = 1.0 / m
			}

			const motorFullRange = 0x5fff
			fl := scaleMotorOutput(frontLeft*scale, motorFullRange)
			fr := scaleMotorOutput(frontRight*scale, motorFullRange)
			bl := scaleMotorOutput(backLeft*scale, motorFullRange)
			br := scaleMotorOutput(backRight*scale, motorFullRange)

			pico.SetMotorSpeeds(fl, fr, bl, br)
		case <-metricsTicker.C:
			battV, _ := pico.BattVolts()
			current, _ := pico.CurrentAmps()
			power, _ := pico.PowerWatts()
			tempC, _ := pico.TemperatureC()
			status, _ := pico.Status()
			fmt.Printf("%.1fC %.2fV %.3fA %.3fW Status=%x\n", tempC, battV, current, power, status)
		}
	}
}

func scaleMotorOutput(value, multiplier float64) int16 {
	multiplied := value * multiplier
	if multiplied <= math.MinInt16 {
		return math.MinInt16
	}
	if multiplied >= math.MaxInt16 {
		return math.MaxInt16
	}
	return int16(multiplied)
}

func applyExpo(value float64, expo float64) float64 {
	absVal := math.Abs(value)
	absExpo := math.Pow(absVal, expo)
	signedExpo := math.Copysign(absExpo, value)
	return signedExpo
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
		events <- event
	}
	return ctx.Err()
}
