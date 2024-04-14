package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"
)

func main() {
	fmt.Println("---- HH tests ----")
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

	hh := hw.StartHeadingHoldMode()
	fmt.Println(
		`Commands:
    t <throttle> <angle>`)

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("\nFailed to read stdin: ", err)
			return
		}
		line = strings.TrimSpace(line)
		parts := strings.Split(line, " ")
		switch parts[0] {
		case "t":
			if len(parts) < 4 {
				fmt.Println("Not enough parameters")
				continue
			}

			throttle, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				fmt.Printf("Failed to parse float: %v\n", err)
				continue
			}
			angle, err := strconv.ParseFloat(parts[2], 64)
			if err != nil {
				fmt.Printf("Failed to parse float: %v\n", err)
				continue
			}
			d, err := time.ParseDuration(parts[3])
			if err != nil {
				fmt.Printf("Failed to parse float: %v\n", err)
				continue
			}
			hh.SetThrottleWithAngle(throttle, angle)
			time.Sleep(d)
			hh.SetThrottle(0)
		case "h":
			if len(parts) < 2 {
				fmt.Println("Not enough parameters")
				continue
			}

			angle, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				fmt.Printf("Failed to parse float: %v\n", err)
			}
			fmt.Printf("Setting heading: %.1f", angle)
			hh.SetHeading(angle)
		}
	}
}
