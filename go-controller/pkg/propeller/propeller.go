package propeller

import (
	"golang.org/x/exp/io/i2c"
)

// DEVICE_REG_MODE1 = 0x00

// PROP_ADDR = 0x42
// MOTOR1_REG = 22
// MOTOR2_REG = 23
// MOTOR3_REG = 24
// MOTOR4_REG = 25
// AUTO_PING_REG = 26
// THROWER_SPD1_REG = 27
// THROWER_SPD2_REG = 28
// THROWER_SRV1_REG = 29
// THROWER_SRV2_REG = 30
// READ_RDY_REG = 31

const (
	PropAddr = 0x42

	RegMotor1 = 22
	RegMotor2 = 23
	RegMotor3 = 24
	RegMotor4 = 25
)

type Propeller struct {
	dev *i2c.Device
}

func New() (*Propeller, error) {
	dev,err  := i2c.Open(&i2c.Devfs{"/dev/i2c-1"}, PropAddr)
	if err != nil {
		return nil, err
	}
	return &Propeller{
		dev: dev,
	}, nil
}

func (p *Propeller) SetSpeeds(left, right int8) error {
	// Clamp to avoid overflow when we negate.
	if left == -128 {
		left = -127
	}
	if right == -128 {
		right = -127
	}
	data := []byte{RegMotor1, byte(-left), byte(-left), byte(right), byte(right)}
	return p.dev.Write(data)
}
