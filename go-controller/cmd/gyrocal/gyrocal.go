package main

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"
)

func main() {
	fmt.Println("---- Gyro calibration ----")
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0))

	// Our global context, we cancel it to trigger shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialise the hardware.
	hw := hardware.New()
	defer func() {
		fmt.Println("Zeroing motors for shut down")
		hw.Shutdown()
		time.Sleep(100 * time.Millisecond)
	}()
	hw.Start(ctx)

	hh := hw.StartYawAndThrottleMode()
	hh.SetYawAndThrottle(0.3, 0, 0)
	time.Sleep(5 * time.Second)
	hh.SetYawAndThrottle(-0.3, 0, 0)
	time.Sleep(5 * time.Second)
}
