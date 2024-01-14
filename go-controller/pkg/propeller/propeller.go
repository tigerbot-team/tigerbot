package propeller

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/kr/pty"
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

	RegServo1 = 27
	RegServo2 = 28
	RegServo3 = 29
	RegServo4 = 30

	RegMotor0 = 22
	RegMotor1 = 23
	RegMotor2 = 24
	RegMotor3 = 25

	RegReadLock = 31
)

type Interface interface {
	SetMotorSpeeds(left, right int8) error
	Close() error
	StartEncoderRead() error
	GetEncoderPositions() (m1, m2 int32, err error)
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

	prop := &Propeller{
		dev: dev,
	}

	err = prop.Flash()
	if err != nil {
		return nil, err
	}

	return prop, nil
}

func (p *Propeller) Flash() error {
	fmt.Println("Flashing the propeller")
	cmd := exec.Command("propman", "/mb3.binary")
	// For some reason, propman requires a TTY, or it reports success but the propeller
	// doesn't actually boot.  Wrap propman with a PTY...
	f, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	fmt.Printf("propman output:\n")
	go io.Copy(os.Stdout, f)
	err = cmd.Wait()
	if err != nil {
		return err
	}
	fmt.Println("Flashed the propeller")
	if err != nil {
		return err
	}
	err = p.enableResetPin()
	if err != nil {
		return err
	}
	// Give propeller time to boot...
	time.Sleep(25 * time.Millisecond)
	return nil
}

func (p *Propeller) enableResetPin() error {
	// Ask the kernel to give us control over the GPIO pin that is connected to the Propeller's reset pin.
	fmt.Println("Taking control of propeller reset pin")
	export, err := os.OpenFile("/sys/class/gpio/export", os.O_WRONLY, 0666)
	defer export.Close()
	if err != nil {
		return err
	}
	_, _ = export.WriteString("17") // Propeller hat reset pin; ignore error, will fail if already exported

	// Reset is active LOW, so we want to drive it HIGH to prevent the propeller from resetting.
	// To do a glitch-free write to the pin, we write "high" to the direction control file.  This
	// ensures that the pin is driven high as soon as it becomes an output.
	fmt.Println("Setting propeller reset pin high (i.e. don't reset)")
	dirn, err := os.OpenFile("/sys/class/gpio/gpio17/direction", os.O_WRONLY, 0666)
	defer dirn.Close()
	_, err = dirn.WriteString("high")
	if err != nil {
		fmt.Println("Failed to drive propeller reset pin", err)
		return err
	}
	return nil
}

func (p *Propeller) Reset() error {
	// Reset is active LOW.
	fmt.Println("Resetting the propeller")
	value, err := os.OpenFile("/sys/class/gpio/gpio17/value", os.O_WRONLY, 0666)
	defer value.Close()
	_, err = value.WriteString("0")
	if err != nil {
		fmt.Println("Failed to drive propeller reset pin: ", err)
		return err
	}
	return nil
}

func (p *Propeller) StartEncoderRead() error {
	data := []byte{RegReadLock, 1}
	return p.writeWithRetries(data)
}

var ErrNotReady = errors.New("encoders not ready")

func (p *Propeller) GetEncoderPositions() (m1, m2 int32, err error) {
	buf := []byte{0}
	err = p.dev.ReadReg(RegReadLock, buf)
	if err != nil {
		return
	}
	if buf[0] != 0 {
		// Not ready to read
		err = ErrNotReady
		return
	}
	buf = make([]byte, 8)
	err = p.dev.ReadReg(4, buf)
	if err != nil {
		return
	}
	m1 = int32(buf[3])<<24 | int32(buf[2])<<16 | int32(buf[1])<<8 | int32(buf[0])
	m2 = -int32(int32(buf[7])<<24 | int32(buf[6])<<16 | int32(buf[5])<<8 | int32(buf[4]))
	return
}

func (p *Propeller) SetMotorSpeeds(left, right int8) error {
	// Clamp all the values for symmetry/to avoid overflow when we negate.
	if left == -128 {
		left = -127
	}
	if right == -128 {
		right = -127
	}
	data := []byte{RegMotor1, byte(-left), byte(right)}
	return p.writeWithRetries(data)
}

func (p *Propeller) Close() error {
	_ = p.Reset()
	return p.dev.Close()
}

//
//func (p *Propeller) SetServo(n int, value uint8) error {
//	var reg byte
//
//	switch n {
//	case 1:
//		reg = RegServo1
//	case 2:
//		reg = RegServo2
//	case 3:
//		reg = RegServo3
//	case 4:
//		reg = RegServo4
//	default:
//		panic(fmt.Errorf("Unknown servo %d", n))
//	}
//
//	fmt.Println("Setting servo", n, "to", value)
//
//	data := []byte{reg, byte(value)}
//	return p.writeWithRetries(data)
//}

func (p *Propeller) writeWithRetries(data []byte) error {
	var err error
	for flashTries := 0; flashTries < 3; flashTries++ {
		for tries := 0; tries < 20; tries++ {
			if err == nil {
				err = p.dev.Write(data)
			} else {
				fmt.Println("Failed to program mux:", err)
			}
			if err == nil {
				if tries > 0 || flashTries > 0 {
					fmt.Println("Successfully programmed propeller after retries")
				}
				return nil
			}
			fmt.Println("Failed to program propeller:", err)
			time.Sleep(1 * time.Millisecond)
			_ = p.dev.Close()
			dev, err := i2c.Open(&i2c.Devfs{"/dev/i2c-1"}, PropAddr)
			if err != nil {
				continue
			}
			p.dev = dev
		}
		// Kill the propeller, in case it's going crazy...
		_ = p.Reset()

		// Then reflash it...
		fmt.Println("Failed to program propeller after retries!!!  Rebooting it!!!", err)
		_ = p.Flash()
	}
	panic("Failed to program or reflash the propeller")
}

type dummyPropeller struct {
}

func (p *dummyPropeller) StartEncoderRead() error {
	return nil
}

func (p *dummyPropeller) GetEncoderPositions() (m1, m2 int32, err error) {
	return
}

func (p *dummyPropeller) SetMotorSpeeds(left, right int8) error {
	fmt.Printf("Dummy propeller setting motors: l=%v r=%v\n", left, right)
	return nil
}

//
//func (p *dummyPropeller) SetServo(n int, value uint8) error {
//	fmt.Printf("Dummy propeller setting servo %d to %d\n", n, value)
//	return nil
//}

func (p *dummyPropeller) Close() error {
	return nil
}
