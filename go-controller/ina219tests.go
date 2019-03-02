package main

import (
	"fmt"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/ina219"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
)

func main() {

	mx, err := mux.New("/dev/i2c-1")
	if err != nil {
		fmt.Println("Failed to open mux", err)
		return
	}

	err = mx.SelectSinglePort(6)
	if err != nil {
		fmt.Println("Failed to select mux port", err)
		return
	}

	ina219A, err := ina219.NewI2C("/dev/i2c-1", ina219.Addr1)
	if err != nil {
		fmt.Println("Failed to open ina219A", err)
		return
	}
	ina219B, err := ina219.NewI2C("/dev/i2c-1", ina219.Addr2)
	if err != nil {
		fmt.Println("Failed to open ina219B", err)
		return
	}

	ina219A.Configure()
	ina219B.Configure()

	for range time.NewTicker(100 * time.Millisecond).C {
		voltage, err := ina219A.ReadBusVoltage()
		fmt.Printf("A: %2fV %v\n", voltage, err)
		current, err := ina219A.ReadBusVoltage()
		fmt.Printf("A: %2fA %v\n", current, err)
		power, err := ina219A.ReadBusVoltage()
		fmt.Printf("A: %2fW %v\n", power, err)
		voltage, err = ina219B.ReadBusVoltage()
		fmt.Printf("B: %2fV %v\n", voltage, err)
		current, err = ina219B.ReadBusVoltage()
		fmt.Printf("B: %2fA %v\n", current, err)
		power, err = ina219B.ReadBusVoltage()
		fmt.Printf("B: %2fW %v\n", power, err)
	}
}
