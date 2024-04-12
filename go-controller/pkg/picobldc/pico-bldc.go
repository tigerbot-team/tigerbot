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

	RegMot0Travel
	RegMot1Travel
	RegMot2Travel
	RegMot3Travel
)

const (
	FrontLeft  = 3
	FrontRight = 0
	BackLeft   = 2
	BackRight  = 1
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

const NumMotors = 4

// Speed is a 16-bit signed fixed point number representing rotations per second.
// 1 sign bit, 5 integral bits, 10 fractional bits.

const SpeedScaleFactor = 1 << 10
const SpeedRPSLSB = 1.0 / SpeedScaleFactor
const SpeedMax = math.MaxInt16 * SpeedRPSLSB

type PerMotorVal[T any] [NumMotors]T

type Interface interface {
	SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error
	RawDistancesTraveled() (raw PerMotorVal[int16], err error)
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
	return p.maybeConfigure(true, false, true)
}

var ErrNotReady = errors.New("Pico-BLDC not ready")

func (p *PicoBLDC) RawDistancesTraveled() (raw PerMotorVal[int16], err error) {
	for m := 0; m < NumMotors; m++ {
		raw[m], err = p.readRegSigned(RegMot0Travel + Register(m))
		if err != nil {
			return
		}
	}
	return
}

func (p *PicoBLDC) SetWatchdog(timeout time.Duration) error {
	if timeout == 0 {
		// Disable.
		p.watchdogEnabled = false
		return p.maybeConfigure(false, false, false)
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
	return p.maybeConfigure(false, false, false)
}

func (p *PicoBLDC) Calibrate() error {
	return p.maybeConfigure(true, false, false)
}

func (p *PicoBLDC) SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error {
	if err := p.maybeConfigure(false, true, false); err != nil {
		return err
	}
	if err := p.writeReg(RegMot1V, uint16(backRight)); err != nil {
		return err
	}
	if err := p.writeReg(RegMot0V, uint16(frontRight)); err != nil {
		return err
	}
	if err := p.writeReg(RegMot3V, uint16(frontLeft)); err != nil {
		return err
	}
	if err := p.writeReg(RegMot2V, uint16(backLeft)); err != nil {
		return err
	}
	return nil
}

func (p *PicoBLDC) Close() error {
	_ = p.maybeConfigure(true, false, false)
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

func (p *PicoBLDC) maybeConfigure(resetMotorSpeeds bool, enableMotors bool, forceCalibration bool) error {
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
	if forceCalibration {
		configWord |= RegCtrlDoCalib
		forceCalibration = false
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
				if status&uint16(RegStatusFault) != 0 {
					fmt.Println("Motor fault detected, are motors powered?")
				}
				lastPrint = time.Now()
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
	p.lastConfigWord = configWord & (^RegCtrlReset) & (^RegCtrlDoCalib) /* Reset flag is not persistent */
	return nil
}

func (p *PicoBLDC) BusVoltage() (float64, error) {
	raw, err := p.readReg(RegBattV)
	if err != nil {
		return 0, err
	}
	v := float64(raw) * BattVLSB
	return v, nil
}

func (p *PicoBLDC) CurrentAmps() (float64, error) {
	raw, err := p.readReg(RegCurrent)
	if err != nil {
		return 0, err
	}
	v := float64(raw) * CurrentLSB
	return v, nil
}

func (p *PicoBLDC) PowerWatts() (float64, error) {
	raw, err := p.readReg(RegPower)
	if err != nil {
		return 0, err
	}
	v := float64(raw) * PowerLSB
	return v, nil
}

func (p *PicoBLDC) TemperatureC() (float64, error) {
	raw, err := p.readReg(RegTemperature)
	if err != nil {
		return 0, err
	}
	v := float64(raw) * TemperatureLSB
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

func (p *PicoBLDC) readRegSigned(reg Register) (int16, error) {
	u, err := p.readReg(reg)
	return int16(u), err
}

type dummyPico struct {
}

func (p *dummyPico) RawDistancesTraveled() (raw PerMotorVal[int16], err error) {

	return
}

func (p *dummyPico) SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error {
	fmt.Printf("Dummy picobldc setting motors: l=%v r=%v\n", frontLeft, frontRight)
	return nil
}

func (p *dummyPico) Close() error {
	return nil
}

func RPSToMotorSpeed(rps float64) int16 {
	scaled := rps * SpeedScaleFactor
	if scaled >= math.MaxInt16 {
		return math.MaxInt16
	}
	if scaled <= math.MinInt16 {
		return math.MinInt16
	}
	return int16(scaled)
}
