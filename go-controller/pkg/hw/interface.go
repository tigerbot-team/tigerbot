package hw

import (
	"sync"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/imu"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"
)

type Hardware struct {
	I2CLock         *sync.Mutex
	Mux             mux.Interface
	Motors          propeller.Interface
	DistanceSensors []*tofsensor.Interface

	SPILock         *sync.Mutex
	IMU             imu.Interface
	ServoController ServoController
}

// TODO Fix up ervo controller iface
type ServoController interface {
	Start(propLock *sync.Mutex, propeller propeller.Interface)
	Stop()

	OnJoystickEvent(event *joystick.Event)
}
