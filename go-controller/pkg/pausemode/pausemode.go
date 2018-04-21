package pausemode

import (
	"context"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
)

type PauseMode struct {
	Propeller propeller.Interface
}

func (t *PauseMode) Name() string {
	return "Pause mode"
}

func (m *PauseMode) StartupSound() string {
	return "/sounds/pausemode.wav"
}

func (t *PauseMode) Start(ctx context.Context) {
	t.Propeller.SetMotorSpeeds(0, 0, 0, 0)
}

func (t *PauseMode) Stop() {
}
