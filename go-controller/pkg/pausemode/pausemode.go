package pausemode

import (
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
	"context"
)

type PauseMode struct {
	Propeller *propeller.Propeller
}

func (t *PauseMode) Name() string{
	return "Pause mode"
}

func (t *PauseMode) Start(ctx context.Context) {
	t.Propeller.SetMotorSpeeds(0,0,0,0)
}

func (t *PauseMode) Stop() {
}
