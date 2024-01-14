package joystick

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"
)

// Button and pad mappings:
//
// Buttons
//
//    Square    = 0
//    Cross     = 1
//    Circle    = 2
//    Triangle  = 3
//    L1        = 4
//    R1        = 5
//    L2        = 6 (also an axis)
//    R2        = 7 (also an axis)
//    Share     = 8
//    Options   = 9
//    L stick   = 10
//    R stick   = 11
//    PS        = 12
//    Pad click = 13
//
// Axes
//
//    D-pad   u/d = 7 (up = -32767; down = +32767)
//            l/r = 6 (left = -32767; right = +32767)
//    L stick u/d = 1 (up = -32767; down = +32767)
//            l/r = 0 (left = -32767; right = +32767)
//    R stick u/d = 4 (up = -32767; down = +32767)
//            l/r = 3 (left = -32767; right = +32767)
//    L2          = 2 (unpressed = -32767; fully-pressed = 32767)
//    R2          = 5 (unpressed = -32767; fully-pressed = 32767)

type EventType uint8

const (
	EventTypeButton = 1
	EventTypeAxis   = 2
)

const (
	ButtonSquare   = 3
	ButtonCross    = 0
	ButtonCircle   = 1
	ButtonTriangle = 2
	ButtonL1       = 4
	ButtonR1       = 5
	ButtonL2       = 6
	ButtonR2       = 7
	ButtonShare    = 8
	ButtonOptions  = 9
	ButtonLStick   = 11
	ButtonRStick   = 12
	ButtonPS       = 10
	//ButtonPadClick =

	AxisLStickX = 0
	AxisLStickY = 1
	AxisRStickX = 3
	AxisRStickY = 4
	AxisDPadX   = 6
	AxisDPadY   = 7
)

func (e EventType) String() string {
	switch e {
	case EventTypeAxis:
		return "axis"
	case EventTypeButton:
		return "button"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(e))
	}
}

type Joystick struct {
	device  *os.File
	readBuf [8]byte

	deviceEpoch    uint32
	wallclockEpoch time.Time
}

type rawEvent struct {
	Time   uint32
	Value  int16
	Type   uint8
	Number uint8
}

type Event struct {
	Time   time.Time
	Value  int16
	Type   EventType
	Number uint8
}

func (e *Event) String() string {
	return fmt.Sprintf("%v(%v)=%v", e.Type, e.Number, e.Value)
}

func NewJoystick(device string) (*Joystick, error) {
	f, err := os.Open(device)
	if err != nil {
		return nil, err
	}
	return &Joystick{
		device: f,
	}, nil
}

func (j *Joystick) ReadEvent() (*Event, error) {
	var rawEvent rawEvent
	err := binary.Read(j.device, binary.LittleEndian, &rawEvent)
	if err != nil {
		return nil, err
	}

	if j.deviceEpoch == 0 {
		j.deviceEpoch = rawEvent.Time
		j.wallclockEpoch = time.Now()
	}

	return &Event{
		Time:   j.wallclockEpoch.Add(time.Duration(rawEvent.Time-j.deviceEpoch) * time.Millisecond),
		Value:  rawEvent.Value,
		Type:   EventType(rawEvent.Type & 0x7f),
		Number: rawEvent.Number,
	}, nil
}

func (j *Joystick) Close() error {
	return j.device.Close()
}
