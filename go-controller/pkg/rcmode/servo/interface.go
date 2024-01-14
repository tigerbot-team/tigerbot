package servo

import "github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"

type ServoSetter interface {
	SetServo(n int, value float64)
}

type ServoController interface {
	Start(servoSetter ServoSetter)
	Stop()

	OnJoystickEvent(event *joystick.Event)
}
