package main

import (
	"fmt"
	"os"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/vl53l5cx"
)

func main() {
	var tof vl53l5cx.Interface
	defer func() {
		_ = tof.Close()
	}()

	tof, err := vl53l5cx.New("/dev/i2c-2")
	if err != nil {
		fmt.Println("Failed to open sensor ", err)
		os.Exit(1)
	}

	err = tof.StartContinuousMeasurements()
	if err != nil {
		fmt.Println("Failed to start continuous measurements", err)
		os.Exit(1)
	}

	for {
		result, err := tof.GetNextContinuousMeasurement()
		if err != nil {
			panic(err)
		}
		fmt.Println(result)
	}
}

//
//var lock sync.Mutex
//var readings hardware.DistanceReadings
//
//func Serve() {
//	fmt.Println("Serving...")
//	tofHandler := func(w http.ResponseWriter, req *http.Request) {
//		var data []byte
//		func() {
//			lock.Lock()
//			defer lock.Unlock()
//			var err error
//			data, err = json.Marshal(readings)
//			if err != nil {
//				panic(err)
//			}
//		}()
//		_, _ = w.Write(data)
//	}
//
//	http.HandleFunc("/tofs", tofHandler)
//	log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
//}
