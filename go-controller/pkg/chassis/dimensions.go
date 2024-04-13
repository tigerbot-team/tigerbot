package chassis

import "math"

const (
	WheelDiameterMM float64 = 70
	WheelCircumMM           = WheelDiameterMM * math.Pi

	BotWidthMM                    = 170
	BotFrontBackWheelCentreDistMM = 190
)

var (
	BotCentreToWheelCentre  = math.Sqrt(math.Pow(BotWidthMM/2, 2) + math.Pow(BotFrontBackWheelCentreDistMM/2, 2))
	WheelTurningCircleDiaMM = math.Pi * BotCentreToWheelCentre * 2
)
