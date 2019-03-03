package servo

import "github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"

type ServoSetter interface {
	SetServo(n int, value uint8)
}

type ServoController interface {
	Start(servoSetter ServoSetter)
	Stop()

	OnJoystickEvent(event *joystick.Event)
}
