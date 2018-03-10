package propeller

import (
	"golang.org/x/exp/io/i2c"
	"fmt"
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

type Interface interface{
	SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int8) error
}

type Propeller struct {
	dev *i2c.Device
}

func Dummy() Interface {
	return &dummyPropeller{}
}

func New() (Interface, error) {
	dev, err := i2c.Open(&i2c.Devfs{"/dev/i2c-1"}, PropAddr)
	if err != nil {
		return nil, err
	}
	return &Propeller{
		dev: dev,
	}, nil
}

func (p *Propeller) SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int8) error {
	// Clamp all the values for symmetry/to avoid overflow when we negate.
	if backLeft == -128 {
		backLeft = -127
	}
	if backRight == -128 {
		backRight = -127
	}
	if frontLeft == -128 {
		frontLeft = -127
	}
	if frontRight == -128 {
		frontRight = -127
	}
	data := []byte{RegMotor1, byte(-backLeft), byte(-frontLeft), byte(frontRight), byte(backRight)}
	return p.dev.Write(data)
}

type dummyPropeller struct {
}

func (p *dummyPropeller) SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int8) error {
	fmt.Printf("Dummy propeller setting motors: fl=%v fr=%v bl=%v br=%v\n", frontLeft, frontRight, backLeft, backRight)
	return nil
}
