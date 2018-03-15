package tofsensor

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/exp/io/i2c"
	"time"
)

const (
	TOFAddr = 0x70
)

type Interface interface {
	Init() error
	Measure() (int, error)
}

type TOFSensor struct {
	dev          *i2c.Device
	stopVariable byte
}

func New() (Interface, error) {
	dev, err := i2c.Open(&i2c.Devfs{"/dev/i2c-1"}, TOFAddr)
	if err != nil {
		return nil, err
	}
	return &TOFSensor{
		dev: dev,
	}, nil
}

const (
	SYSRANGE_START = 0x00

	SYSTEM_THRESH_HIGH = 0x0C
	SYSTEM_THRESH_LOW  = 0x0E

	SYSTEM_SEQUENCE_CONFIG         = 0x01
	SYSTEM_RANGE_CONFIG            = 0x09
	SYSTEM_INTERMEASUREMENT_PERIOD = 0x04

	SYSTEM_INTERRUPT_CONFIG_GPIO = 0x0A

	GPIO_HV_MUX_ACTIVE_HIGH = 0x84

	SYSTEM_INTERRUPT_CLEAR = 0x0B

	RESULT_INTERRUPT_STATUS = 0x13
	RESULT_RANGE_STATUS     = 0x14

	RESULT_CORE_AMBIENT_WINDOW_EVENTS_RTN = 0xBC
	RESULT_CORE_RANGING_TOTAL_EVENTS_RTN  = 0xC0
	RESULT_CORE_AMBIENT_WINDOW_EVENTS_REF = 0xD0
	RESULT_CORE_RANGING_TOTAL_EVENTS_REF  = 0xD4
	RESULT_PEAK_SIGNAL_RATE_REF           = 0xB6

	ALGO_PART_TO_PART_RANGE_OFFSET_MM = 0x28

	I2C_SLAVE_DEVICE_ADDRESS = 0x8A

	MSRC_CONFIG_CONTROL = 0x60

	PRE_RANGE_CONFIG_MIN_SNR           = 0x27
	PRE_RANGE_CONFIG_VALID_PHASE_LOW   = 0x56
	PRE_RANGE_CONFIG_VALID_PHASE_HIGH  = 0x57
	PRE_RANGE_MIN_COUNT_RATE_RTN_LIMIT = 0x64

	FINAL_RANGE_CONFIG_MIN_SNR                  = 0x67
	FINAL_RANGE_CONFIG_VALID_PHASE_LOW          = 0x47
	FINAL_RANGE_CONFIG_VALID_PHASE_HIGH         = 0x48
	FINAL_RANGE_CONFIG_MIN_COUNT_RATE_RTN_LIMIT = 0x44

	PRE_RANGE_CONFIG_SIGMA_THRESH_HI = 0x61
	PRE_RANGE_CONFIG_SIGMA_THRESH_LO = 0x62

	PRE_RANGE_CONFIG_VCSEL_PERIOD      = 0x50
	PRE_RANGE_CONFIG_TIMEOUT_MACROP_HI = 0x51
	PRE_RANGE_CONFIG_TIMEOUT_MACROP_LO = 0x52

	SYSTEM_HISTOGRAM_BIN                  = 0x81
	HISTOGRAM_CONFIG_INITIAL_PHASE_SELECT = 0x33
	HISTOGRAM_CONFIG_READOUT_CTRL         = 0x55

	FINAL_RANGE_CONFIG_VCSEL_PERIOD       = 0x70
	FINAL_RANGE_CONFIG_TIMEOUT_MACROP_HI  = 0x71
	FINAL_RANGE_CONFIG_TIMEOUT_MACROP_LO  = 0x72
	CROSSTALK_COMPENSATION_PEAK_RATE_MCPS = 0x20

	MSRC_CONFIG_TIMEOUT_MACROP = 0x46

	SOFT_RESET_GO2_SOFT_RESET_N = 0xBF
	IDENTIFICATION_MODEL_ID     = 0xC0
	IDENTIFICATION_REVISION_ID  = 0xC2

	OSC_CALIBRATE_VAL = 0xF8

	GLOBAL_CONFIG_VCSEL_WIDTH        = 0x32
	GLOBAL_CONFIG_SPAD_ENABLES_REF_0 = 0xB0
	GLOBAL_CONFIG_SPAD_ENABLES_REF_1 = 0xB1
	GLOBAL_CONFIG_SPAD_ENABLES_REF_2 = 0xB2
	GLOBAL_CONFIG_SPAD_ENABLES_REF_3 = 0xB3
	GLOBAL_CONFIG_SPAD_ENABLES_REF_4 = 0xB4
	GLOBAL_CONFIG_SPAD_ENABLES_REF_5 = 0xB5

	GLOBAL_CONFIG_REF_EN_START_SELECT   = 0xB6
	DYNAMIC_SPAD_NUM_REQUESTED_REF_SPAD = 0x4E
	DYNAMIC_SPAD_REF_EN_START_OFFSET    = 0x4F
	POWER_MANAGEMENT_GO1_POWER_FORCE    = 0x80

	VHV_CONFIG_PAD_SCL_SDA__EXTSUP_HV = 0x89

	ALGO_PHASECAL_LIM            = 0x30
	ALGO_PHASECAL_CONFIG_TIMEOUT = 0x30
)

func (p *TOFSensor) Init() error {
	// "Set I2C standard mode"
	p.WriteReg8(0x88, 0x00)

	p.WriteReg8(0x80, 0x01)
	p.WriteReg8(0xFF, 0x01)
	p.WriteReg8(0x00, 0x00)

	var buf [1]byte
	p.dev.ReadReg(0x91, buf[:])
	p.stopVariable = buf[0]

	p.WriteReg8(0x00, 0x01)
	p.WriteReg8(0xFF, 0x00)
	p.WriteReg8(0x80, 0x00)

	// disable SIGNAL_RATE_MSRC (bit 1) and SIGNAL_RATE_PRE_RANGE (bit 4) limit checks
	//p.WriteReg8(MSRC_CONFIG_CONTROL, this->readRegister(MSRC_CONFIG_CONTROL) | 0x12);

	// set final range signal rate limit to 0.25 MCPS (million counts per second)
	//this->setSignalRateLimit(0.25);

	p.WriteReg8(SYSTEM_SEQUENCE_CONFIG, 0xFF)

	p.WriteReg8(SYSTEM_INTERRUPT_CONFIG_GPIO, 0x04);
	// active low
	v, _ := p.ReadReg8(GPIO_HV_MUX_ACTIVE_HIGH)
	p.WriteReg8(GPIO_HV_MUX_ACTIVE_HIGH, v & 0xef);
	p.WriteReg8(SYSTEM_INTERRUPT_CLEAR, 0x01);
	return nil
}

func (p *TOFSensor) WriteReg8(reg, value byte) error {
	return p.dev.WriteReg(reg, []byte{value})
}

func (p *TOFSensor) Measure() (int, error) {
	p.WriteReg8(0x80, 0x01)
	p.WriteReg8(0xFF, 0x01)
	p.WriteReg8(0x00, 0x00)
	p.WriteReg8(0x91, p.stopVariable)
	p.WriteReg8(0x00, 0x01)
	p.WriteReg8(0xFF, 0x00)
	p.WriteReg8(0x80, 0x00)

	p.WriteReg8(SYSRANGE_START, 0x01)

	// "Wait until start bit has been cleared"
	var buf [1]byte
	for {
		err := p.dev.ReadReg(SYSRANGE_START, buf[:])
		if err != nil {
			fmt.Println("Failed to read from device")
			continue
		}
		if buf[0]&0x01 == 0 {
			break
		}
		time.Sleep(1 * time.Microsecond)
	}

	//for {
	//	err := p.dev.ReadReg(RESULT_INTERRUPT_STATUS, buf[:])
	//	if err != nil {
	//		fmt.Println("Failed to read from device")
	//		continue
	//	}
	//	if buf[0]&0x07 == 0 {
	//		break
	//	}
	//	time.Sleep(1 * time.Microsecond)
	//}

	r1, err := p.ReadReg8(RESULT_RANGE_STATUS + 10)
	r2, err := p.ReadReg8(RESULT_RANGE_STATUS + 11)

	p.dev.WriteReg(SYSTEM_INTERRUPT_CLEAR, []byte{0x01})

	return int(r1)<<8 | int(r2), err
}

func (p *TOFSensor) ReadReg16(addr byte) (uint16, error) {
	var buf [2]byte
	err := p.dev.ReadReg(addr, buf[:])
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

func (p *TOFSensor) ReadReg8(addr byte) (byte, error) {
	var buf [1]byte
	err := p.dev.ReadReg(addr, buf[:])
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}
