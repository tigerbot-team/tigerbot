package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/picobldc"
)

// Measurements for movement in a given direction for 5s.
type measurements struct {
	// Wheel rotations.
	rotations picobldc.PerMotorVal[float64]

	// Measured displacement straight ahead.
	aheadMM float64

	// Measured displacement sideways to the left.
	leftMM float64
}

var table [73][2]measurements

var scanner *bufio.Scanner

func init() {
	scanner = bufio.NewScanner(os.Stdin)
}

func getDisplacements() (float64, float64) {
	fmt.Println("Enter straight ahead displacement (mm):")
	if !scanner.Scan() {
		panic(scanner.Err())
	}
retry:
	ahead, err := strconv.ParseFloat(scanner.Text(), 64)
	if err != nil {
		fmt.Printf("error: %v, please try again:\n", err)
		goto retry
	}

	fmt.Println("Enter sideways (left +tive) displacement (mm):")
	if !scanner.Scan() {
		panic(scanner.Err())
	}
	left, err := strconv.ParseFloat(scanner.Text(), 64)
	if err != nil {
		fmt.Printf("error: %v, please try again:\n", err)
		goto retry
	}
	return ahead, left
}

func main() {
	fmt.Println("---- Movement Calibration ----")
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0))

	// Our global context, we cancel it to trigger shutdown.
	ctx, cancel := context.WithCancel(context.Background())

	// Initialise the hardware.
	hw := hardware.New()
	defer func() {
		fmt.Println("Zeroing motors for shut down")
		hw.Shutdown()
		time.Sleep(100 * time.Millisecond)
	}()
	hw.Start(ctx)

	hh := hw.StartHeadingHoldMode()
	throttleSpeed := 10 // Just need a slow walking speed here.

	for i := 0; i < 36; i++ {
		angle := [2]int{i*5 - 180, i * 5}
		index := [2]int{i, i + 36}
		fmt.Printf("Measurements %v/36: angles %v, %v...\n", i+1, angle[0], angle[1])
		for dir := 0; dir < 2; dir++ {
			for j := 0; j < 2; j++ {
				fmt.Printf("%v/4:\n", 1+j+2*dir)
				startRotations := hw.AccumulatedRotations()
				hh.SetThrottleWithAngle(float64(throttleSpeed), float64(angle[dir]))
				time.Sleep(time.Duration(4*(j+1)) * time.Second)
				hw.StopMotorControl()
				endRotations := hw.AccumulatedRotations()
				table[index[dir]][j].aheadMM, table[index[dir]][j].leftMM = getDisplacements()
				for k := range endRotations {
					table[index[dir]][j].rotations[k] = endRotations[k] - startRotations[k]
				}
			}
			printRow(index[dir], table[index[dir]])
		}
	}

	fmt.Println("")
	fmt.Println("Whole table:")
	for i := range table {
		printRow(i, table[i])
	}
}

func printRow(index int, row [2]measurements) {
	fmt.Printf("table[%v] = %#v\n", index, row)
}
