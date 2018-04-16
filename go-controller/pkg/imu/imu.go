package imu

import (
	"fmt"
	"math"

	"golang.org/x/exp/io/i2c"
)

const (
	IMUAddr = 0x68

	RegSampleRateDiv = 25
	RegConfig        = 26
	RegGyroConf      = 27
	RegGyroXOffset   = 19
	RegFIFOEnable    = 35
	RegGyroX         = 67 // 16 bits
	RegUserCtl       = 106
	RegFIFOCount     = 114 // 16 bits
	RegFIFORW        = 116 // n-bytes

	GyroRange = 2 // 1000 dps
)

type Interface interface {
	Configure() error
	Calibrate() error
	ReadGyroX() int16
	ReadFIFO() []int16
	ResetFIFO()
	DegreesPerLSB() float64
}

type IMU struct {
	dev *i2c.Device
}

func New(deviceFile string) (Interface, error) {
	dev, err := i2c.Open(&i2c.Devfs{deviceFile}, IMUAddr)
	if err != nil {
		return nil, err
	}
	return &IMU{
		dev: dev,
	}, nil
}

func (m *IMU) Configure() error {
	// Set GYRO range
	err := m.dev.WriteReg(RegGyroConf, []byte{GyroRange << 3})
	if err != nil {
		return err
	}
	// Set DLPF Fs=1KHz
	err = m.dev.WriteReg(RegConfig, []byte{1})
	if err != nil {
		return err
	}
	// Divide output rate
	err = m.dev.WriteReg(RegSampleRateDiv, []byte{9})
	if err != nil {
		return err
	}
	// Enable write of gyro X to FIFO
	err = m.dev.WriteReg(RegFIFOEnable, []byte{1 << 6})
	return err
}

func (m *IMU) DegreesPerLSB() float64 {
	return 1000.0 / math.MaxInt16
}

func (m *IMU) Calibrate() error {
	// Now calibrate...
	fmt.Println("Calibrating gyro")
	m.dev.WriteReg(RegGyroXOffset, []byte{0, 0})

	for i := 0; i < 100; i++ {
		m.ReadGyroX()
	}

	var sum float64
	const n = 1000
	for i := 0; i < n; i++ {
		x := m.ReadGyroX()
		sum -= float64(x)
	}
	offset := sum / n
	fmt.Println("Offset", offset)
	scaledOffset := offset / 4 * math.Pow(2, GyroRange)
	fmt.Println("Scaled offset", scaledOffset)
	scaledOffsetInt := int16(scaledOffset)
	fmt.Println("Scaled offset int", scaledOffsetInt)
	m.dev.WriteReg(RegGyroXOffset, []byte{byte(scaledOffsetInt >> 8), byte(scaledOffset)})

	return nil
}

func (m *IMU) ReadGyroX() int16 {
	return m.Read16(RegGyroX)
}

func (m *IMU) ResetFIFO() {
	m.dev.WriteReg(RegUserCtl, []byte{1<<6 | 1<<2})
}

func (m *IMU) ReadFIFO() []int16 {
	var count int16
	for count == 0 {
		count = m.Read16(RegFIFOCount) & 0xfff
	}
	var buf [512]byte
	result := make([]int16, count/2)
	m.dev.ReadReg(RegFIFORW, buf[:count])
	for i := 0; i < int(count/2); i++ {
		highByte := buf[i*2]
		lowByte := buf[i*2+1]
		result[i] = int16(highByte)<<8 | int16(lowByte)
	}
	return result
}

func (m *IMU) Read16(reg byte) int16 {
	var buf [2]byte
	m.dev.ReadReg(reg, buf[:])
	return int16(buf[0])<<8 | int16(buf[1])
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
