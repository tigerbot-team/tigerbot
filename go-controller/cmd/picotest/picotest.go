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
	lastRot := distance.AccumulatedRotations()
	invl := 500 * time.Millisecond
	ticker := time.NewTicker(invl)

	for {
		loopStart := time.Now()
		_ = pico.SetMotorSpeeds(picobldc.RPSToMotorSpeed(1), 0, 0, 0)
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
		fmt.Printf("Accumulated: fl=%7f fr=%7f bl=%7f br=%7f\n",
			rot[picobldc.FrontLeft], rot[picobldc.FrontRight], rot[picobldc.BackLeft], rot[picobldc.BackRight])
		dt := float64(invl) / float64(time.Second)
		fmt.Printf("Speed RPS:   fl=%7f fr=%7f bl=%7f br=%7f\n",
			(rot[picobldc.FrontLeft]-lastRot[picobldc.FrontLeft])/dt,
			(rot[picobldc.FrontRight]-lastRot[picobldc.FrontRight])/dt,
			(rot[picobldc.BackLeft]-lastRot[picobldc.BackLeft])/dt,
			(rot[picobldc.BackRight]-lastRot[picobldc.BackRight])/dt,
		)
		lastRot = rot
		<-ticker.C
	}
}
