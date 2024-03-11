package picobldc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

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
	PicoAddr = 0x42
)

type Register byte

const (
	RegCtrl Register = iota
	RegStatus
	RegWatchdogTimeout
	RegFaultCount

	RegMot0V
	RegMot1V
	RegMot2V
	RegMot3V

	RegMot0Calib
	RegMot1Calib
	RegMot2Calib
	RegMot3Calib

	RegBattV // LSB=4mV
	RegCurrent
	RegPower

	RegTemperature // LSB = 0.01C
)

const (
	BattVLSB       = 0.004
	CurrentLSB     = 0.0001831054688
	PowerLSB       = CurrentLSB * 20
	TemperatureLSB = 0.01
)

const (
	RegCtrlEnableI2CControl uint16 = 1 << iota
	RegCtrlRun
	RegCtrlDoCalib
	RegCtrlReset
	RegCtrlWatchdogEnable
)

type StatusFlag uint16

const (
	RegStatusFault StatusFlag = 1 << iota
	RegStatusCalibDone
	RegStatusWatchdogExpired
)

type Interface interface {
	SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error
	Close() error
}

type PicoBLDC struct {
	dev *i2c.Device

	lastConfigWord  uint16
	lastConfigTime  time.Time
	watchdogEnabled bool
}

func Dummy() Interface {
	return &dummyPico{}
}

var i2cBus = &i2c.Devfs{Dev: "/dev/i2c-1"}

func New() (*PicoBLDC, error) {
	dev, err := i2c.Open(i2cBus, PicoAddr)
	if err != nil {
		return nil, err
	}

	pico := &PicoBLDC{
		dev: dev,
	}

	return pico, nil
}

var _ Interface = (*PicoBLDC)(nil)

func (p *PicoBLDC) Reset() error {
	return p.maybeConfigure(true, false)
}

var ErrNotReady = errors.New("Pico-BLDC not ready")

func (p *PicoBLDC) GetEncoderPositions() (frontLeft, frontRight, backLeft, backRight int16, err error) {
	// TODO
	return
}

func (p *PicoBLDC) SetWatchdog(timeout time.Duration) error {
	if timeout == 0 {
		// Disable.
		p.watchdogEnabled = false
		return p.maybeConfigure(false, false)
	}

	ms := timeout.Milliseconds()
	if ms > math.MaxUint16 {
		ms = math.MaxUint16
	}
	err := p.writeReg(RegWatchdogTimeout, uint16(ms))
	if err != nil {
		return err
	}

	p.watchdogEnabled = true
	return p.maybeConfigure(false, false)
}

func (p *PicoBLDC) SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error {
	if err := p.maybeConfigure(false, true); err != nil {
		return err
	}
	if err := p.writeReg(RegMot0V, uint16(backRight)); err != nil {
		return err
	}
	if err := p.writeReg(RegMot1V, uint16(frontRight)); err != nil {
		return err
	}
	if err := p.writeReg(RegMot2V, uint16(frontLeft)); err != nil {
		return err
	}
	if err := p.writeReg(RegMot3V, uint16(backLeft)); err != nil {
		return err
	}
	return nil
}

func (p *PicoBLDC) Close() error {
	_ = p.Reset()
	return p.dev.Close()
}

func (p *PicoBLDC) writeWithRetries(data []byte) error {
	var err error
	for tries := 0; tries < 20; tries++ {
		err = p.dev.Write(data)
		if err == nil {
			if tries > 0 {
				fmt.Println("Successfully programmed Pico-BLDC after retries")
			}
			return nil
		}
		fmt.Println("Failed to write to Pico-BLDC:", err)
		time.Sleep(1 * time.Millisecond)
		_ = p.dev.Close()
		dev, err := i2c.Open(i2cBus, PicoAddr)
		if err != nil {
			continue
		}
		p.dev = dev
	}
	panic("Failed to write to Pico-BLDC")
}

func (p *PicoBLDC) maybeConfigure(resetMotorSpeeds bool, enableMotors bool) error {
	// Figure out if the config word has changed.
	var configWord uint16 = RegCtrlEnableI2CControl
	if resetMotorSpeeds {
		configWord |= RegCtrlReset
	}
	if enableMotors {
		configWord |= RegCtrlRun
	}
	if p.watchdogEnabled {
		configWord |= RegCtrlWatchdogEnable
	}

	if configWord == p.lastConfigWord && time.Since(p.lastConfigTime) < 100*time.Millisecond {
		// Skip writing config if we've done it recently.
		return nil
	}

	if p.lastConfigWord == 0 {
		// First time.  Figure out calibration...
		calib, err := p.readReg(RegMot3Calib)
		if err != nil {
			return err
		}
		if calib == 0 {
			// Calibration register empty, do a calibration.
			// FIXME, won't work on actual robot; needs to be on blocks.
			fmt.Println("Pico-BLDC not calibrated, running calibration...")
			configWord |= RegCtrlDoCalib
		}
	}

	if err := p.writeReg(RegCtrl, configWord); err != nil {
		return err
	}

	if configWord&RegCtrlDoCalib != 0 {
		// Wait for calibration to finish.
		var lastPrint time.Time
		for {
			status, err := p.readReg(RegStatus)
			if err != nil {
				fmt.Printf("Pico: failed to read status register: %v\n", err)
			}
			if status&uint16(RegStatusCalibDone) != 0 {
				break
			}
			if time.Since(lastPrint) > time.Second {
				fmt.Printf("Waiting for calibration to finish... Status=%x\n", status)
			}
		}

		fmt.Printf("Calibration words:")
		for r := RegMot0Calib; r <= RegMot3Calib; r++ {
			v, err := p.readReg(r)
			if err != nil {
				return err
			}
			fmt.Printf(" %04x", v)
		}
		fmt.Print("\n")
	}

	if err := p.writeReg(RegStatus, uint16(RegStatusCalibDone)); err != nil {
		return err
	}

	p.lastConfigTime = time.Now()
	p.lastConfigWord = configWord & (^RegCtrlReset) /* Reset flag is not persistent */
	return nil
}

func (p *PicoBLDC) BattVolts() (float32, error) {
	raw, err := p.readReg(RegBattV)
	if err != nil {
		return 0, err
	}
	v := float32(raw) * BattVLSB
	return v, nil
}

func (p *PicoBLDC) CurrentAmps() (float32, error) {
	raw, err := p.readReg(RegCurrent)
	if err != nil {
		return 0, err
	}
	v := float32(raw) * CurrentLSB
	return v, nil
}

func (p *PicoBLDC) PowerWatts() (float32, error) {
	raw, err := p.readReg(RegPower)
	if err != nil {
		return 0, err
	}
	v := float32(raw) * PowerLSB
	return v, nil
}

func (p *PicoBLDC) TemperatureC() (float32, error) {
	raw, err := p.readReg(RegTemperature)
	if err != nil {
		return 0, err
	}
	v := float32(raw) * TemperatureLSB
	return v, nil
}

func (p *PicoBLDC) Status() (StatusFlag, error) {
	raw, err := p.readReg(RegStatus)
	if err != nil {
		return 0, err
	}
	return StatusFlag(raw), nil
}

func (p *PicoBLDC) writeReg(reg Register, value uint16) error {
	return p.writeWithRetries([]byte{byte(reg), byte(value >> 8), byte(value)})
}

func (p *PicoBLDC) readReg(reg Register) (uint16, error) {
	var buf [2]byte
	err := p.dev.ReadReg(byte(reg), buf[:])
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

type dummyPico struct {
}

func (p *dummyPico) GetEncoderPositions() (m1, m2 int32, err error) {
	return
}

func (p *dummyPico) SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error {
	fmt.Printf("Dummy picobldc setting motors: l=%v r=%v\n", frontLeft, frontRight)
	return nil
}

func (p *dummyPico) Close() error {
	return nil
}
