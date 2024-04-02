package main

import (
	"fmt"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/picobldc"
)

func main() {
	fmt.Println("Pico-BLDC test program")
	pico, err := picobldc.New()
	if err != nil {
		panic(err)
	}
	fmt.Println("Created PicoBLDC object. Enabling watchdog...")

	if err := pico.Calibrate(); err != nil {
		panic(err)
	}

	if err := pico.SetWatchdog(time.Second); err != nil {
		panic(err)
	}
	fmt.Println("Watchdog enabled.")

	distance := picobldc.NewDistanceTracker(pico)
	for {
		loopStart := time.Now()
		_ = pico.SetMotorSpeeds(500, -1000, 1500, -2000)
		setTime := time.Since(loopStart)
		fmt.Printf("Motor update time: %s\n", setTime.Round(100*time.Microsecond))
		battV, _ := pico.BusVoltage()
		current, _ := pico.CurrentAmps()
		power, _ := pico.PowerWatts()
		tempC, _ := pico.TemperatureC()
		status, _ := pico.Status()
		fmt.Printf("%.1fC %.2fV %.3fA %.3fW Status=%x\n", tempC, battV, current, power, status)
		_ = distance.Poll()
		rot := distance.AccumulatedRotations()
		fmt.Printf("fl=%7f fr=%7f bl=%7f br=%7f\n", rot[picobldc.FrontLeft], rot[picobldc.FrontRight], rot[picobldc.BackLeft], rot[picobldc.BackRight])
		time.Sleep(500 * time.Millisecond)
	}
}
