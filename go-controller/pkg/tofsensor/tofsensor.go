package tofsensor


/*
#cgo CFLAGS: -I ../../../VL53L0X_1.0.2/Api/core/inc/
#cgo CFLAGS: -I ../../../VL53L0X_rasp/platform/inc/
#cgo LDFLAGS: -L ../../../VL53L0X_rasp/bin -lVL53L0X_Rasp

#include <vl53l0x_api.h>
*/
import "C"


const (
	TOFAddr = 0x70
)

type Interface interface {
	Init() error
	Measure() (int, error)
}

type TOFSensor struct {
}

func New() (Interface, error) {

	return nil, nil
}

func (p *TOFSensor) Init() error {
	return nil
}

func (p *TOFSensor) Measure() (int, error) {
	return 0, nil
}
