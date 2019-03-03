package mux

import (
	"fmt"

	"golang.org/x/exp/io/i2c"
)

const (
	MuxAddr = 0x70

	BusTOFForwardLeft  = 0
	BusTOFForwardRight = 1
	BusTOFLeftFront    = 2
	BusTOFLeftRear     = 3
	BusTOFRightFront   = 4
	BusTOFRightRear    = 5

	BusOthers = 6

	BusPropeller = 7
)

type Interface interface {
	DisableAllPorts() error
	SelectSinglePort(num int) error
	SelectMultiplePorts(i byte) error
	Close() error
}

type Mux struct {
	dev *i2c.Device
}

func New(deviceFile string) (Interface, error) {
	dev, err := i2c.Open(&i2c.Devfs{deviceFile}, MuxAddr)
	if err != nil {
		return nil, err
	}
	return &Mux{
		dev: dev,
	}, nil
}

func (p *Mux) SelectSinglePort(num int) error {
	data := []byte{1 << uint(num)}
	return p.dev.Write(data)
}
func (p *Mux) SelectMultiplePorts(i byte) error {
	data := []byte{i}
	return p.dev.Write(data)
}

func (p *Mux) DisableAllPorts() error {
	data := []byte{0}
	return p.dev.Write(data)
}

func (p *Mux) Close() error {
	return p.dev.Close()
}

func Dummy() Interface {
	return &dummyMux{}
}

type dummyMux struct {
}

func (p *dummyMux) SelectSinglePort(num int) error {
	fmt.Printf("Dummy Mux setting port=%d\n", num)
	return nil
}

func (p *dummyMux) DisableAllPorts() error {
	fmt.Printf("Dummy Mux disabling all ports\n")
	return nil
}

func (p *dummyMux) SelectMultiplePorts(i byte) error {
	return nil
}

func (p *dummyMux) Close() error {
	return nil
}
