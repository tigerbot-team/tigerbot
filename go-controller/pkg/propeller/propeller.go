package propeller

import i2c2 "github.com/d2r2/go-i2c"

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

	RegMotor1 = 0x22
	RegMotor2 = 0x23
	RegMotor3 = 0x24
	RegMotor4 = 0x25
)

type Propeller struct {
	i2c *i2c2.I2C
}

func New() (*Propeller, error) {
	i2c, err := i2c2.NewI2C(PropAddr, 1)
	if err != nil {
		return nil, err
	}
	return &Propeller{
		i2c: i2c,
	}, nil
}

func (p *Propeller) SetSpeeds(left, right byte) error {
	// Clamp to avoid overflow when we negate.
	if left == -128 {
		left = -127
	}
	if right == -128 {
		right = -127
	}
	data := []byte{4, -left, -left, right, right}

	err := p.i2c.WriteRegU8(RegMotor1, uint8(-left))
	if err != nil {
		return err
	}
	err = p.i2c.WriteRegU8(RegMotor2, uint8(-left))
	if err != nil {
		return err
	}
	err = p.i2c.WriteRegU8(RegMotor3, uint8(right))
	if err != nil {
		return err
	}
	err = p.i2c.WriteRegU8(RegMotor4, uint8(right))
	if err != nil {
		return err
	}
	return nil
}
