package pausemode

import (
	"context"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
)

func New(hw hardware.Interface) *PauseMode {
	return &PauseMode{
		hw: hw,
	}
}

type PauseMode struct {
	Propeller propeller.Interface
	hw        hardware.Interface
}

func (t *PauseMode) Name() string {
	return "Pause mode"
}

func (m *PauseMode) StartupSound() string {
	return "/sounds/pausemode.wav"
}

func (t *PauseMode) Start(ctx context.Context) {
	t.hw.StopMotorControl()
}

func (t *PauseMode) Stop() {
}
