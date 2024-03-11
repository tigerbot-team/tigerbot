package pausemode

import (
	"context"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/picobldc"
)

func New(hw hardware.Interface) *PauseMode {
	return &PauseMode{
		hw: hw,
	}
}

type PauseMode struct {
	Propeller picobldc.Interface
	hw        hardware.Interface
}

func (t *PauseMode) Name() string {
	return "PAUSE MODE"
}

func (m *PauseMode) StartupSound() string {
	return "/sounds/pausemode.wav"
}

func (t *PauseMode) Start(ctx context.Context) {
	t.hw.StopMotorControl()
}

func (t *PauseMode) Stop() {
}
