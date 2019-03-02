package main

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
)

func main() {

	fmt.Println("GOMAXPROCS: ", runtime.GOMAXPROCS(10))

	mx, err := mux.New("/dev/i2c-1")
	if err != nil {
		fmt.Println("Failed to open mux", err)
		return
	}

	var tofs []tofsensor.Interface
	defer func() {
		for _, tof := range tofs {
			_ = tof.Close()
		}
	}()
	for _, port := range []int{
		0, 1, 3, 4, 5,
	} {
		fmt.Println("Intiialising ToF ", port)

		err := mx.SelectSinglePort(port)
		if err != nil {
			fmt.Println("Failed to select mux port", err)
			return
		}

		tof, err := tofsensor.New("/dev/i2c-1", 0x29, byte(0x30+port))
		if err != nil {
			tof, err = tofsensor.New("/dev/i2c-1", byte(0x30+port))
			if err != nil {
				fmt.Println("Failed to open sensor", err)
				return
			}
		}

		err = tof.StartContinuousMeasurements()
		if err != nil {
			fmt.Println("Failed to start continuous measurements", err)
			return
		}
		tofs = append(tofs, tof)
	}

	mx.SelectMultiplePorts(0x3f)

	var l sync.Mutex
	readSensors := func() {
		// Read the sensors
		msgs := []string{}
		var wg sync.WaitGroup
		start := time.Now()
		for j, tof := range tofs {
			j := j
			tof := tof
			wg.Add(1)
			go func() {
				reading := "-"
				readingInMM, err := tof.GetNextContinuousMeasurement()
				if err == tofsensor.ErrMeasurementInvalid {
					reading = "<invalid>"
				} else if err != nil {
					reading = "<failed>"
				} else {
					reading = fmt.Sprintf("%dmm", readingInMM)
				}
				l.Lock()
				msgs = append(msgs, fmt.Sprintf("%d: %s  ", j, reading))
				l.Unlock()
				wg.Done()
			}()
		}
		wg.Wait()
		duration := time.Since(start)
		sort.Strings(msgs)
		fmt.Println(strings.Join(msgs, ""), " ", duration)
	}

	for range time.NewTicker(100 * time.Millisecond).C {
		readSensors()
	}
}
