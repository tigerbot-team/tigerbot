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

	if err := pico.SetWatchdog(time.Second); err != nil {
		panic(err)
	}
	fmt.Println("Watchdog enabled.")

	for {
		_ = pico.SetMotorSpeeds(1000, 2000, 3000, 4000)
		battV, _ := pico.BattVolts()
		current, _ := pico.CurrentAmps()
		power, _ := pico.PowerWatts()
		tempC, _ := pico.TemperatureC()
		status, _ := pico.Status()
		fmt.Printf("%.1fC %.2fV %.3fA %.3fW Status=%x\n", tempC, battV, current, power, status)
		time.Sleep(500 * time.Millisecond)
	}
}
