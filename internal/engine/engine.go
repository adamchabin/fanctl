package engine

import (
	"sort"

	"fanctl/internal/config"
)

func AggregateTemperatures(sensorIDs []string, tempMap map[string]float64, mode config.AggregationMode) (float64, bool) {
	var validTemps []float64

	for _, id := range sensorIDs {
		if val, exists := tempMap[id]; exists {
			validTemps = append(validTemps, val)
		}
	}

	if len(validTemps) == 0 {
		return 0, false
	}

	if mode == config.AggAvg {
		var sum float64
		for _, t := range validTemps {
			sum += t
		}
		return sum / float64(len(validTemps)), true
	}

	maxTemp := validTemps[0]
	for _, t := range validTemps {
		if t > maxTemp {
			maxTemp = t
		}
	}
	return maxTemp, true
}

func CalculatePWM(currentTemp float64, rule config.FanRule, lastPWM int) int {
	points := rule.Curve
	if len(points) == 0 {
		return rule.MinPWM
	}

	sorted := make([]config.CurvePoint, len(points))
	copy(sorted, points)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Temp < sorted[j].Temp
	})

	var rawPWM int

	if currentTemp <= sorted[0].Temp {
		rawPWM = sorted[0].PWM
	} else if currentTemp >= sorted[len(sorted)-1].Temp {
		rawPWM = sorted[len(sorted)-1].PWM
	} else {
		for i := 0; i < len(sorted)-1; i++ {
			p1 := sorted[i]
			p2 := sorted[i+1]

			if currentTemp >= p1.Temp && currentTemp <= p2.Temp {
				ratio := (currentTemp - p1.Temp) / (p2.Temp - p1.Temp)
				rawPWM = int(float64(p1.PWM) + ratio*float64(p2.PWM-p1.PWM))
				break
			}
		}
	}

	_ = lastPWM

	if rawPWM < rule.MinPWM {
		return rule.MinPWM
	}
	if rule.MaxPWM > 0 && rawPWM > rule.MaxPWM {
		return rule.MaxPWM
	}

	return rawPWM
}
