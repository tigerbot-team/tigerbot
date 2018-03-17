package mux

import (
	"golang.org/x/exp/io/i2c"
	"fmt"
)

const (
	MuxAddr = 0x70

	BusTOF1 = 1
	BusTOF2 = 2
	BusTOF3 = 6
)

type Interface interface {
	DisableAllPorts() error
	SelectSinglePort(num int) error
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
