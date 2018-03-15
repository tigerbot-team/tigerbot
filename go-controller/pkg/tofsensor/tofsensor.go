package tofsensor

import "golang.org/x/exp/io/i2c"




/*
#cgo CFLAGS: -I ../../../VL53L0X_1.0.2/Api/core/inc
#cgo LDFLAGS: -L . -lclibrary

#include "clibrary.h"

int callOnMeGo_cgo(int in); // Forward declaration.
#include "vl53l0x_api.h"
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
	dev          *i2c.Device
	stopVariable byte
}

func New() (Interface, error) {


	return &TOFSensor{
		dev: dev,
	}, nil
}

func (p *TOFSensor) Init() error {
	return nil
}

func (p *TOFSensor) Measure() (int, error) {

}
