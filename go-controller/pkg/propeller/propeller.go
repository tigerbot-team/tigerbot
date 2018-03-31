package propeller

import (
	"fmt"
	"golang.org/x/exp/io/i2c"
	"os"
	"os/exec"
	"time"
	"github.com/kr/pty"
	"io"
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

type Interface interface {
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
	err =  p.enableResetPin()
	if err != nil {
		return err
	}
	// Give propeller time to boot...
	time.Sleep(25*time.Millisecond)
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
		fmt.Println("Failed to drive propeller reset pin")
		return err
	}
	return nil
}

func (p *Propeller) Reset() error {
	// Reset is active LOW.
	fmt.Println("Resetting the propeller")
	value, err := os.OpenFile("/sys/class/gpio/gpio17/value", os.O_WRONLY, 0666)
	defer value.Close()
	_, err = value.WriteString("low")
	if err != nil {
		fmt.Println("Failed to drive propeller reset pin")
		return err
	}
	return nil
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
	var err error
	for flashTries := 0; flashTries < 3; flashTries++ {
		for tries := 0; tries < 20; tries++ {
			err = p.dev.Write(data)
			if err == nil {
				if tries > 0 || flashTries > 0 {
					fmt.Println("Successfully programmed propeller after retries")
				}
				return nil
			}
			fmt.Println("Failed to program propeller:", err)
			time.Sleep(1 * time.Millisecond)
			p.dev.Close()
			dev, err := i2c.Open(&i2c.Devfs{"/dev/i2c-1"}, PropAddr)
			if err != nil {
				continue
			}
			p.dev = dev
		}
		// Kill the propeller, in case it's going crazy...
		p.Reset()

		// Then reflash it...
		fmt.Println("Failed to program propeller after retries!!!  Rebooting it!!!", err)
		p.Flash()
	}
	panic("Failed to program or reflash the propeller")
}

type dummyPropeller struct {
}

func (p *dummyPropeller) SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int8) error {
	fmt.Printf("Dummy propeller setting motors: fl=%v fr=%v bl=%v br=%v\n", frontLeft, frontRight, backLeft, backRight)
	return nil
}
