package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
)

func main() {
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
		0, 1, 2, 3, 4, 5,
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
				fmt.Println("Failed to open sensor, skipping ", port, " ", err)
				continue
			}
		}

		err = tof.StartContinuousMeasurements()
		if err != nil {
			fmt.Println("Failed to start continuous measurements", err)
			continue
		}
		tofs = append(tofs, tof)
	}

	err = mx.SelectMultiplePorts(0x3f)
	if err != nil {
		fmt.Println("Failed to select mux port", err)
		return
	}

	go Serve()

	var l sync.Mutex
	readSensors := func() {
		// Read the sensors
		msgs := []string{}
		var wg sync.WaitGroup
		start := time.Now()
		var currentReads hardware.DistanceReadings
		currentReads.CaptureTime = time.Now()
		currentReads.Readings = make([]hardware.Reading, len(tofs))
		for j, tof := range tofs {
			j := j
			tof := tof
			wg.Add(1)
			go func() {
				defer wg.Done()
				reading := "-"
				readingInMM, err := tof.GetNextContinuousMeasurement()
				currentReads.Readings[j] = hardware.Reading{readingInMM, err}
				if readingInMM == tofsensor.RangeTooFar {
					reading = ">2000mm"
				} else if err != nil {
					reading = "<failed>"
				} else {
					reading = fmt.Sprintf("%dmm", readingInMM)
				}
				l.Lock()
				msgs = append(msgs, fmt.Sprintf("%d: %s  ", j, reading))
				l.Unlock()
			}()
		}
		wg.Wait()
		lock.Lock()
		readings = currentReads
		lock.Unlock()
		duration := time.Since(start)
		sort.Strings(msgs)
		fmt.Println(strings.Join(msgs, ""), " ", duration)
	}

	for range time.NewTicker(100 * time.Millisecond).C {
		readSensors()
	}
}

var lock sync.Mutex
var readings hardware.DistanceReadings

func Serve() {
	fmt.Println("Serving...")
	tofHandler := func(w http.ResponseWriter, req *http.Request) {
		var data []byte
		func() {
			lock.Lock()
			defer lock.Unlock()
			var err error
			data, err = json.Marshal(readings)
			if err != nil {
				panic(err)
			}
		}()
		_, _ = w.Write(data)
	}

	http.HandleFunc("/tofs", tofHandler)
	log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
}
