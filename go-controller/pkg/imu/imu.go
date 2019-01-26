package imu

import (
	"fmt"
	"math"

	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/conn/spi"
	"periph.io/x/periph/conn/spi/spireg"
	"periph.io/x/periph/host"

	"golang.org/x/exp/io/i2c"
)

const (
	IMUAddr = 0x68

	RegSampleRateDiv = 25
	RegConfig        = 26
	RegGyroConf      = 27
	RegGyroYOffset   = 21
	RegFIFOEnable    = 35
	RegGyroY         = 69 // 16 bits
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

type port interface {
	// Read reads len(buf) bytes from the device.
	ReadReg(reg byte, buf []byte) error
	WriteReg(reg byte, buf []byte) (err error)
}

type IMU struct {
	dev        port
	disableI2C bool
}

func NewI2C(deviceFile string) (Interface, error) {
	dev, err := i2c.Open(&i2c.Devfs{deviceFile}, IMUAddr)
	if err != nil {
		return nil, err
	}
	return &IMU{
		dev: dev,
	}, nil
}

func NewSPI(deviceFile string) (Interface, error) {
	// Make sure periph is initialized.
	if _, err := host.Init(); err != nil {
		return nil, err
	}

	// Use spireg SPI port registry to find the SPI bus.
	p, err := spireg.Open(deviceFile)
	if err != nil {
		return nil, err
	}

	// Convert the spi.Port into a spi.Conn so it can be used for communication.
	c, err := p.Connect(physic.KiloHertz*1000, spi.Mode3, 8)
	if err != nil {
		return nil, err
	}

	// Wrap the SPI connection in our interface.
	dev := SPIAdapter{c: c}

	return &IMU{
		dev:        &dev,
		disableI2C: true,
	}, nil
}

type SPIAdapter struct {
	c spi.Conn

	r, w []byte
}

const W = 0x00
const R = 0x80

func (s *SPIAdapter) ReadReg(reg byte, buf []byte) error {
	// The read and write buffers need to be as long as the whole transaction.
	bufLen := 1 + len(buf)
	s.ensureBuf(bufLen)
	// We write the address byte, then read back the response.
	addrByte := R | reg
	s.w[0] = addrByte
	err := s.c.Tx(s.w[:bufLen], s.r[:bufLen])
	if err != nil {
		return err
	}
	// The response will come back only after the first byte is sent, ignore the first byte that we read.
	copy(buf, s.r[1:])
	return nil
}

func (s *SPIAdapter) WriteReg(reg byte, buf []byte) (err error) {
	// The read and write buffers need to be as long as the whole transaction.
	bufLen := 1 + len(buf)
	s.ensureBuf(bufLen)
	// We write the address byte, then the data.
	addrByte := W | reg
	s.w[0] = addrByte
	copy(s.w[1:], buf)
	err = s.c.Tx(s.w[:bufLen], s.r[:bufLen])
	return
}

func (s *SPIAdapter) ensureBuf(l int) {
	if len(s.r) < l {
		s.w = make([]byte, l)
		s.r = make([]byte, l)
	} else {
		for i := 0; i < l; i++ {
			s.w[i] = 0
			s.r[i] = 0
		}
	}
}

func (m *IMU) Configure() error {
	if m.disableI2C {
		err := m.dev.WriteReg(RegUserCtl, []byte{0x10})
		if err != nil {
			return err
		}
	}
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
	// Enable write of gyro Y to FIFO
	err = m.dev.WriteReg(RegFIFOEnable, []byte{1 << 5})
	return err
}

func (m *IMU) DegreesPerLSB() float64 {
	return 1000.0 / math.MaxInt16
}

func (m *IMU) Calibrate() error {
	// Now calibrate...
	fmt.Println("Calibrating gyro")
	m.dev.WriteReg(RegGyroYOffset, []byte{0, 0})

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
	m.dev.WriteReg(RegGyroYOffset, []byte{byte(scaledOffsetInt >> 8), byte(scaledOffset)})

	return nil
}

func (m *IMU) ReadGyroX() int16 {
	return m.Read16(RegGyroY)
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
