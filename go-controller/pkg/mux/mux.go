package mux

import (
	"fmt"

	"golang.org/x/exp/io/i2c"
)

const (
	MuxAddr = 0x70

	BusTOFForward    = 0
	BusTOFFrontLeft  = 2
	BusTOFFrontRight = 6
	BusTOFSideLeft   = 7
	BusTOFSideRight  = 1
)

type Interface interface {
	DisableAllPorts() error
	SelectSinglePort(num int) error
	SelectMultiplePorts(i byte) error
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
