package main

import (
	"fmt"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/ina219"
)

func main() {
	ina219A, err := ina219.NewI2C("/dev/i2c-1", ina219.Addr1)
	if err != nil {
		fmt.Println("Failed to open ina219A", err)
		return
	}
	//ina219B, err := ina219.NewI2C("/dev/i2c-1", ina219.Addr2)
	//if err != nil {
	//	fmt.Println("Failed to open ina219B", err)
	//	return
	//}

	err = ina219A.Configure(0.1, 2.0)
	if err != nil {
		fmt.Println("Failed to configure ina219B", err)
		return
	}

	//ina219B.Configure(0.05, 1.0)

	for range time.NewTicker(500 * time.Millisecond).C {
		voltage, err := ina219A.BusVoltage()
		fmt.Printf("A: %.2fV %v ", voltage, err)
		current, err := ina219A.CurrentAmps()
		fmt.Printf("A: %.3fA %v ", current, err)
		power, err := ina219A.PowerWatts()
		fmt.Printf("A: %.3fW %v\n", power, err)
		//voltage, err = ina219B.BusVoltage()
		//fmt.Printf("B: %.2fV %v ", voltage, err)
		//current, err = ina219B.CurrentAmps()
		//fmt.Printf("B: %.3fA %v ", current, err)
		//power, err = ina219B.PowerWatts()
		//fmt.Printf("B: %.3fW %v\n", power, err)
	}
}
