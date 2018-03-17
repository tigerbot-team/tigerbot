package tofsensor

/*
#cgo CFLAGS: -I ../../../VL53L0X_1.0.2/Api/core/inc/
#cgo CFLAGS: -I ../../../VL53L0X_rasp/platform/inc/
#cgo LDFLAGS: -L ../../../VL53L0X_rasp/bin -lVL53L0X_Rasp

#include <stdlib.h>
#include "vl53l0x_api.h"
*/
import "C"
import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	TOFAddr = 0x70
)

var (
	ErrI2CInitFailed  = errors.New("I2C init failed")
	ErrDataInitFailed = errors.New("data init failed")
	ErrMeasurementFailed = errors.New("measurement failed")
	ErrMeasurementInvalid = errors.New("measurement invalid")
)

type Interface interface {
	Measure() (int, error)
	Close() error
}

type TOFSensor struct {
	deviceFileName *C.char
	device         *C.VL53L0X_Dev_t
}

func New(device string, addr byte) (Interface, error) {
	tof := &TOFSensor{}
	
	var status C.VL53L0X_Error = C.VL53L0X_ERROR_NONE
	defer func() {
		if status != C.VL53L0X_ERROR_NONE {
			tof.Close()
		}
	}()

	// Not sure if it's necessary to alloc the struct from the C heap or not
	// but it's certainly safe.
	tof.device = (*C.VL53L0X_Dev_t)(C.malloc(C.sizeof_VL53L0X_Dev_t))
	tof.device.I2cDevAddr = C.uchar(addr)

	tof.deviceFileName = C.CString(device)
	tof.device.fd = C.VL53L0X_i2c_init(tof.deviceFileName, C.int(tof.device.I2cDevAddr))
	if tof.device.fd < 0 {
		return nil, ErrI2CInitFailed
	}

	status = C.VL53L0X_DataInit(tof.device)
	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}

	status = C.VL53L0X_StaticInit(tof.device)
	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}

	var (
		vhvSettings C.uint8_t
		phaseCal    C.uint8_t
	)
	status = C.VL53L0X_PerformRefCalibration(tof.device, &vhvSettings, &phaseCal)
	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}

	var (
		refSpadCount    C.uint32_t
		isApertureSpads C.uint8_t
	)
	status = C.VL53L0X_PerformRefSpadManagement(tof.device, &refSpadCount, &isApertureSpads)
	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}

	status = C.VL53L0X_SetDeviceMode(tof.device, C.VL53L0X_DEVICEMODE_SINGLE_RANGING)
	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}

	status = C.VL53L0X_SetLimitCheckEnable(tof.device, C.VL53L0X_CHECKENABLE_SIGMA_FINAL_RANGE, 1)

	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetLimitCheckEnable(tof.device, C.VL53L0X_CHECKENABLE_SIGNAL_RATE_FINAL_RANGE, 1)

	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetLimitCheckValue(tof.device, C.VL53L0X_CHECKENABLE_SIGNAL_RATE_FINAL_RANGE, (C.FixPoint1616_t)(6554))

	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetLimitCheckValue(tof.device, C.VL53L0X_CHECKENABLE_SIGMA_FINAL_RANGE, (C.FixPoint1616_t)(60*65536))

	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetMeasurementTimingBudgetMicroSeconds(tof.device, 33000)

	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetVcselPulsePeriod(tof.device, C.VL53L0X_VCSEL_PERIOD_PRE_RANGE, 18)

	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetVcselPulsePeriod(tof.device, C.VL53L0X_VCSEL_PERIOD_FINAL_RANGE, 14)

	if status != C.VL53L0X_ERROR_NONE {
		return nil, ErrDataInitFailed
	}
	return tof, nil
}

func (p *TOFSensor) Close() error {
	if p.device.fd != 0 {
		syscall.Close(int(p.device.fd))
		p.device.fd = 0
	}
	if p.device != nil {
		C.free(unsafe.Pointer(p.device))
		p.device = nil
	}
	return nil
}

func (p *TOFSensor) Measure() (int, error) {
	var meas C.VL53L0X_RangingMeasurementData_t
	status := C.VL53L0X_PerformSingleRangingMeasurement(p.device, &meas)
	if status != C.VL53L0X_ERROR_NONE {
		return 0, ErrMeasurementFailed
	}
	if meas.RangeStatus != 0 {
		return 0, ErrMeasurementInvalid
	}
	return int(meas.RangeMilliMeter), nil
}
