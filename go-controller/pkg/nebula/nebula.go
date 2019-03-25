package nebula

import (
	"fmt"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rainbow"
)

func FindBestMatch(targets []*rainbow.HSVRange, averageHue []byte, hueUsed []bool) (int, []int) {
	var (
		minCost  int
		minOrder []int
	)
	for ic, choiceHue := range averageHue {
		if hueUsed[ic] {
			continue
		}
		choiceCost := calculateCost(targets[0], choiceHue)
		choiceOrder := []int{ic}
		if len(targets) > 1 {
			// This is not the last target.
			hueUsedCopy := make([]bool, len(hueUsed))
			copy(hueUsedCopy, hueUsed)
			hueUsedCopy[ic] = true
			nextCost, nextOrder := FindBestMatch(targets[1:], averageHue, hueUsedCopy)
			choiceCost = choiceCost + nextCost
			choiceOrder = append(choiceOrder, nextOrder...)
		}
		fmt.Printf("cost %v for order %v\n", choiceCost, choiceOrder)
		if (minOrder == nil) || (choiceCost < minCost) {
			minCost = choiceCost
			minOrder = choiceOrder
		}
	}
	return minCost, minOrder
}

func calculateCost(targetHSVRange *rainbow.HSVRange, choiceHue byte) int {
	var hueDelta byte = 0
	if targetHSVRange.HueMin <= targetHSVRange.HueMax {
		// Non-wrapped hue range.
		if choiceHue < targetHSVRange.HueMin {
			hueDelta = targetHSVRange.HueMin - choiceHue
		} else if choiceHue > targetHSVRange.HueMax {
			hueDelta = choiceHue - targetHSVRange.HueMax
		}
	} else {
		// Wrapped hue range.
		if (choiceHue < targetHSVRange.HueMin) && (choiceHue > targetHSVRange.HueMax) {
			delta1 := targetHSVRange.HueMin - choiceHue
			delta2 := choiceHue - targetHSVRange.HueMax
			if delta1 < delta2 {
				hueDelta = delta1
			} else {
				hueDelta = delta2
			}
		}
	}
	return int(hueDelta) * int(hueDelta)
}
