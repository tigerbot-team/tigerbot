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
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
)

const (
	TOFAddr = 0x70

	// RangeTooFar is the range in mm that we return if the measurement was invalid, which
	// typically means that the sensor got no response because the surface was too far away.
	RangeTooFar = 2001
)

var (
	ErrI2CInitFailed     = errors.New("I2C init failed")
	ErrDataInitFailed    = errors.New("data init failed")
	ErrMeasurementFailed = errors.New("measurement failed")
)

type Interface interface {
	StartContinuousMeasurements() error
	GetNextContinuousMeasurement() (int, error)

	DoSingleMeasurement() (int, error)

	Close() error
}

type MuxedTOFSensor struct {
	Port int
	tof  Interface
	mux  mux.Interface
}

func NewMuxed(device string, addr byte, mux mux.Interface, muxPort int) (Interface, error) {
	err := mux.SelectSinglePort(muxPort)
	if err != nil {
		return nil, err
	}
	tof, err := New(device, addr)
	if err != nil {
		return nil, err
	}
	muxed := MuxedTOFSensor{
		Port: muxPort,
		tof:  tof,
		mux:  mux,
	}
	return &muxed, nil
}

func (m *MuxedTOFSensor) DoSingleMeasurement() (int, error) {
	err := m.mux.SelectSinglePort(m.Port)
	if err != nil {
		return 0, err
	}
	return m.tof.DoSingleMeasurement()
}

func (m *MuxedTOFSensor) StartContinuousMeasurements() error {
	err := m.mux.SelectSinglePort(m.Port)
	if err != nil {
		return err
	}
	return m.tof.StartContinuousMeasurements()
}

func (m *MuxedTOFSensor) GetNextContinuousMeasurement() (int, error) {
	err := m.mux.SelectSinglePort(m.Port)
	if err != nil {
		return 0, err
	}
	return m.tof.GetNextContinuousMeasurement()
}

func (m *MuxedTOFSensor) Close() error {
	return m.tof.Close()
}

type TOFSensor struct {
	deviceFileName *C.char
	device         *C.VL53L0X_Dev_t
}

func New(device string, addr ...byte) (Interface, error) {
	tof := &TOFSensor{}

	var status C.VL53L0X_Error
	status = C.VL53L0X_ERROR_NONE

	defer func() {
		if status != C.VL53L0X_ERROR_NONE {
			tof.Close()
		}
	}()

	// Not sure if it's necessary to alloc the struct from the C heap or not
	// but it's certainly safe.
	tof.device = (*C.VL53L0X_Dev_t)(C.malloc(C.sizeof_VL53L0X_Dev_t))
	tof.device.I2cDevAddr = C.uchar(addr[0])

	tof.deviceFileName = C.CString(device)
	tof.device.fd = C.VL53L0X_i2c_init(tof.deviceFileName, C.int(tof.device.I2cDevAddr))
	if tof.device.fd < 0 {
		return nil, ErrI2CInitFailed
	}

	if len(addr) > 1 {
		fmt.Printf("Moving ToF from address %#x to %#x\n", addr[0], addr[1])
		status := C.VL53L0X_SetDeviceAddress(tof.device, C.uchar(addr[1])<<1)
		if status != C.VL53L0X_ERROR_NONE {
			fmt.Printf("Failed to move ToF from addreess %#x to %#x; checking if it's already there...\n", addr[0], addr[1])
		}

		_ = syscall.Close(int(tof.device.fd))

		tof.device.I2cDevAddr = C.uchar(addr[1])
		tof.device.fd = C.VL53L0X_i2c_init(tof.deviceFileName, C.int(tof.device.I2cDevAddr))
		if tof.device.fd < 0 {
			return nil, ErrI2CInitFailed
		}
	}

	status = C.VL53L0X_DataInit(tof.device)
	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_DataInit failed: ", status)
		return nil, ErrDataInitFailed
	}

	status = C.VL53L0X_StaticInit(tof.device)
	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_StaticInit failed: ", status)
		return nil, ErrDataInitFailed
	}

	var (
		vhvSettings C.uint8_t
		phaseCal    C.uint8_t
	)
	status = C.VL53L0X_PerformRefCalibration(tof.device, &vhvSettings, &phaseCal)
	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_PerformRefCalibration failed: ", status)
		return nil, ErrDataInitFailed
	}

	var (
		refSpadCount    C.uint32_t
		isApertureSpads C.uint8_t
	)
	status = C.VL53L0X_PerformRefSpadManagement(tof.device, &refSpadCount, &isApertureSpads)
	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_PerformRefSpadManagement failed: ", status)
		return nil, ErrDataInitFailed
	}

	status = C.VL53L0X_SetDeviceMode(tof.device, C.VL53L0X_DEVICEMODE_SINGLE_RANGING)
	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_SetDeviceMode failed: ", status)
		return nil, ErrDataInitFailed
	}

	status = C.VL53L0X_SetLimitCheckEnable(tof.device, C.VL53L0X_CHECKENABLE_SIGMA_FINAL_RANGE, 1)

	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_SetLimitCheckEnable failed: ", status)
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetLimitCheckEnable(tof.device, C.VL53L0X_CHECKENABLE_SIGNAL_RATE_FINAL_RANGE, 1)

	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_SetLimitCheckEnable failed: ", status)
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetLimitCheckValue(tof.device, C.VL53L0X_CHECKENABLE_SIGNAL_RATE_FINAL_RANGE, (C.FixPoint1616_t)(6554))

	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_SetLimitCheckValue failed: ", status)
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetLimitCheckValue(tof.device, C.VL53L0X_CHECKENABLE_SIGMA_FINAL_RANGE, (C.FixPoint1616_t)(60*65536))

	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_SetLimitCheckValue failed: ", status)
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetMeasurementTimingBudgetMicroSeconds(tof.device, 33000)

	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_SetMeasurementTimingBudgetMicroSeconds failed: ", status)
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetVcselPulsePeriod(tof.device, C.VL53L0X_VCSEL_PERIOD_PRE_RANGE, 18)

	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_SetVcselPulsePeriod failed: ", status)
		return nil, ErrDataInitFailed
	}
	status = C.VL53L0X_SetVcselPulsePeriod(tof.device, C.VL53L0X_VCSEL_PERIOD_FINAL_RANGE, 14)

	if status != C.VL53L0X_ERROR_NONE {
		fmt.Println("VL53L0X_SetVcselPulsePeriod failed: ", status)
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

func (p *TOFSensor) DoSingleMeasurement() (int, error) {
	var meas C.VL53L0X_RangingMeasurementData_t
	status := C.VL53L0X_PerformSingleRangingMeasurement(p.device, &meas)
	if status != C.VL53L0X_ERROR_NONE {
		return 0, ErrMeasurementFailed
	}
	if meas.RangeStatus != 0 {
		return RangeTooFar, nil
	}
	return int(meas.RangeMilliMeter), nil
}

func (p *TOFSensor) StartContinuousMeasurements() error {
	status := C.VL53L0X_SetDeviceMode(p.device, C.VL53L0X_DEVICEMODE_CONTINUOUS_RANGING)
	if status != C.VL53L0X_ERROR_NONE {
		return ErrDataInitFailed
	}
	status = C.VL53L0X_StartMeasurement(p.device)
	if status != C.VL53L0X_ERROR_NONE {
		return ErrDataInitFailed
	}
	C.VL53L0X_ClearInterruptMask(p.device,
		C.VL53L0X_REG_SYSTEM_INTERRUPT_GPIO_NEW_SAMPLE_READY)
	return nil
}

var (
	ErrWaitFailed = errors.New("failed to wait")
	ErrTimeout    = errors.New("timed out")
)

func (p *TOFSensor) GetNextContinuousMeasurement() (int, error) {
	var ready C.uint8_t
	start := time.Now()
	for {
		status := C.VL53L0X_GetMeasurementDataReady(p.device, &ready)
		if status != C.VL53L0X_ERROR_NONE {
			return 0, ErrWaitFailed
		}
		if ready == 1 {
			break
		}
		time.Sleep(100 * time.Microsecond)
		if time.Since(start) > time.Second {
			return 0, ErrTimeout
		}
	}

	var meas C.VL53L0X_RangingMeasurementData_t
	status := C.VL53L0X_GetRangingMeasurementData(p.device, &meas)
	if status != C.VL53L0X_ERROR_NONE {
		return 0, ErrMeasurementFailed
	}
	C.VL53L0X_ClearInterruptMask(p.device,
		C.VL53L0X_REG_SYSTEM_INTERRUPT_GPIO_NEW_SAMPLE_READY)
	if meas.RangeStatus != 0 {
		return RangeTooFar, nil
	}
	return int(meas.RangeMilliMeter), nil
}
