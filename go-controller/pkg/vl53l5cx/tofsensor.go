package vl53l5cx

/*
#include <stdlib.h>
#include "vl53l5cx_api.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"time"
	"unsafe"
)

const (
	TOFAddr = 0x52

	// RangeTooFar is the range in mm that we return if the measurement was invalid, which
	// typically means that the sensor got no response because the surface was too far away.
	RangeTooFar = 2001
)

type Interface interface {
	StartContinuousMeasurements() error
	GetNextContinuousMeasurement() ([]int, error)

	Close() error
}

type VL53L5CX struct {
	deviceFileName *C.char
	device         *C.VL53L5CX_Configuration
}

func New(device string) (Interface, error) {
	tof := &VL53L5CX{}

	success := false
	tof.device = (*C.VL53L5CX_Configuration)(C.malloc(C.sizeof_VL53L5CX_Configuration))

	{
		// Initialise the platform-specific code.
		rc := C.vl53l5cx_comms_init(&tof.device.platform, C.CString(device))
		fmt.Println("vl53l5cx_comms_init = ", rc)
		if rc != 0 {
			return nil, fmt.Errorf("VL53L5CX comms init failed rc=%d", int(rc))
		}
		defer func() {
			if success {
				return
			}
			fmt.Println("Failed to init VL53L5CX, closing I2C.")
			rc := C.vl53l5cx_comms_close(&tof.device.platform)
			fmt.Println("vl53l5cx_comms_close = ", rc)
		}()
	}
	{
		var alive C.uint8_t
		rc := C.vl53l5cx_is_alive(tof.device, &alive)
		fmt.Println("vl53l5cx_is_alive = ", rc)
		if rc != 0 {
			return nil, fmt.Errorf("VL53L5CX is alive failed rc=%d", int(rc))
		}
		if alive != 0 {
			return nil, fmt.Errorf("VL53L5CX not found on bus")
		}
	}
	{
		rc := C.vl53l5cx_init(tof.device)
		fmt.Println("vl53l5cx_init = ", rc)
		if rc != 0 {
			return nil, fmt.Errorf("VL53L5CX init failed rc=%d", int(rc))
		}
	}
	success = true
	return tof, nil
}

func (p *VL53L5CX) Close() error {
	if p.device == nil {
		return nil
	}
	rc := C.vl53l5cx_comms_close(&p.device.platform)
	fmt.Println("vl53l5cx_comms_close RC = ", rc)
	C.free(unsafe.Pointer(p.device))
	p.device = nil
	return nil
}

func (p *VL53L5CX) StartContinuousMeasurements() error {
	status := C.vl53l5cx_start_ranging(p.device)
	if status != 0 {
		return fmt.Errorf("failed to enable ranging rc =%d", status)
	}
	return nil
}

var (
	ErrWaitFailed = errors.New("failed to wait")
	ErrTimeout    = errors.New("timed out")
)

func (p *VL53L5CX) GetNextContinuousMeasurement() ([]int, error) {
	startTime := time.Now()
	for {
		var ready C.uint8_t
		rc := C.vl53l5cx_check_data_ready(p.device, &ready)
		if rc != 0 {
			return nil, fmt.Errorf("failed to read from TOF (vl53l5cx_check_data_ready); rc = %d", int(rc))
		}
		if ready != 0 {
			break
		}
		if time.Since(startTime) > time.Second {
			return nil, ErrTimeout
		}
	}

	var results C.VL53L5CX_ResultsData
	rc := C.vl53l5cx_get_ranging_data(p.device, &results)
	if rc != 0 {
		return nil, fmt.Errorf("failed to read from TOF (vl53l5cx_get_ranging_data); rc = %d", int(rc))
	}

	output := make([]int, 16)
	for i := 0; i < 16; i++ {
		output[i] = int(results.distance_mm[i])
	}
	// TODO Read target status too
	return output, nil
}
