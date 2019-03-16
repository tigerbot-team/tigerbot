package ina219

import (
	"fmt"

	"golang.org/x/exp/io/i2c"
)

const (
	Addr1 = 0x41
	Addr2 = 0x44

	RegConfig      = 0
	RegShuntV      = 1
	RegBusV        = 2
	RegPower       = 3
	RegCurrent     = 4
	RegCalibration = 5

	BusVoltageLSB = 0.004
)

type Interface interface {
	Configure(shuntOhms float64, maxCurrent float64) error
	ReadBusVoltage() (float64, error)
	ReadCurrent() (float64, error)
	ReadPower() (float64, error)
}

type port interface {
	// Read reads len(buf) bytes from the device.
	ReadReg(reg byte, buf []byte) error
	WriteReg(reg byte, buf []byte) (err error)
}

type INA219 struct {
	currentLSB float64
	dev        port
}

func NewI2C(deviceFile string, addr int) (Interface, error) {
	dev, err := i2c.Open(&i2c.Devfs{deviceFile}, addr)
	if err != nil {
		return nil, err
	}
	return &INA219{
		dev: dev,
	}, nil
}

func (m *INA219) Configure(shuntOhms float64, maxCurrent float64) error {
	// Write Calibration register
	m.currentLSB = maxCurrent / (1 << 15)
	cval := CalculateCalibrationValue(m.currentLSB, shuntOhms)
	fmt.Printf("INA219 calibration value: 0x%x\n", cval)
	err := m.dev.WriteReg(RegCalibration, []byte{byte(cval >> 8), byte(cval)})
	return err
}

func (m *INA219) ReadBusVoltage() (float64, error) {
	raw, err := m.Read16(RegBusV)
	shifted := raw >> 3
	return float64(shifted) * BusVoltageLSB, err
}

func (m *INA219) ReadCurrent() (float64, error) {
	raw, err := m.Read16(RegCurrent)
	return float64(raw) * m.currentLSB, err
}

func (m *INA219) ReadPower() (float64, error) {
	raw, err := m.Read16(RegPower)
	return float64(raw) * m.currentLSB * 20, err
}

func (m *INA219) Read16(reg byte) (uint16, error) {
	var buf [2]byte
	err := m.dev.ReadReg(reg, buf[:])
	return uint16(buf[0])<<8 | uint16(buf[1]), err
}

func CalculateCalibrationValue(currentLSB float64, shuntOhms float64) int16 {
	return int16(0.04096 / (currentLSB * shuntOhms))
}

//
//
//func Dummy() Interface {
//	return &dummyIMU{}
//}
//
//type dummyIMU struct {
//}
//
//func (p *dummyIMU) SelectSinglePort(num int) error {
//	fmt.Printf("Dummy IMU setting port=%d\n", num)
//	return nil
//}
//
//func (p *dummyIMU) DisableAllPorts() error {
//	fmt.Printf("Dummy IMU disabling all ports\n")
//	return nil
//}
