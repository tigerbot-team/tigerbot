package pca9685

import (
	"fmt"
	"time"

	"golang.org/x/exp/io/i2c"
)

const (
	DefaultAddr = 0x40

	RegMode1 = 0x00
	RegMode2 = 0x01

	// Each PWM output has two 16-bit (low byte first) registers.
	// First register is the on time, second is the off time.
	RegLEDBase = 0x06

	RegPreScale = 0xfe // Pre-scaler for PWM frequency.
	RegTestMode = 0xff

	PWMPeriod = 20 * time.Millisecond

	ServoMinPulseDuration = 1000 * time.Microsecond
	ServoMaxPulseDuration = 2000 * time.Microsecond

	PWMMax = 4095

	ServoMinPWM = float64(PWMMax * ServoMinPulseDuration / PWMPeriod)
	ServoMaxPWM = float64(PWMMax * ServoMaxPulseDuration / PWMPeriod)
)

type Interface interface {
	Configure() error
	SetServo(port int, value float64) error
	SetPWM(port int, value float64) error
	Close() error
}

type PCA9685 struct {
	dev *i2c.Device
}

func New(deviceFile string) (Interface, error) {
	dev, err := i2c.Open(&i2c.Devfs{deviceFile}, DefaultAddr)
	if err != nil {
		return nil, err
	}
	return &PCA9685{
		dev: dev,
	}, nil
}

func (p *PCA9685) Configure() (err error) {
	// Put device to sleep.
	err = p.dev.WriteReg(RegMode1, []byte{0x11})
	if err != nil {
		return
	}
	// Update pre-scaler for 50Hz.
	err = p.dev.WriteReg(RegPreScale, []byte{0x79})
	if err != nil {
		return
	}
	// Trigger a reset
	err = p.dev.WriteReg(RegMode1, []byte{0x01})
	if err != nil {
		return
	}
	// Required delay after reset.
	time.Sleep(1 * time.Millisecond)
	// Enable.
	err = p.dev.WriteReg(RegMode1, []byte{0x81})
	return
}

func (p *PCA9685) SetServo(port int, value float64) error {
	if port < 0 || port > 15 {
		fmt.Println("Servo port out of range: ", port)
		return nil
	}
	if value < 0 {
		value = 0
	} else if value > 1 {
		value = 1
	}

	pwmValue := uint16(ServoMinPWM + value*(ServoMaxPWM-ServoMinPWM))
	addr := RegLEDBase + port*4

	return p.dev.WriteReg(byte(addr), []byte{0, 0, byte(pwmValue & 0xff), byte(pwmValue >> 8)})
}

func (p *PCA9685) SetPWM(port int, value float64) error {
	if port < 0 || port > 15 {
		fmt.Println("PWM port out of range: ", port)
		return nil
	}
	if value < 0 {
		value = 0
	} else if value > 1 {
		value = 1
	}

	pwmValue := uint16(PWMMax * value)
	addr := RegLEDBase + port*4

	return p.dev.WriteReg(byte(addr), []byte{0, 0, byte(pwmValue & 0xff), byte(pwmValue >> 8)})
}

func (p *PCA9685) Close() error {
	return p.dev.Close()
}

func Dummy() Interface {
	return &dummyServo{}
}

type dummyServo struct {
}

func (*dummyServo) Configure() error {
	return nil
}

func (*dummyServo) SetServo(port int, value float64) error {
	return nil
}

func (*dummyServo) SetPWM(port int, value float64) error {
	return nil
}

func (*dummyServo) Close() error {
	return nil
}
