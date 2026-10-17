package weights

// Congestion converts an observed edge speed into a congestion ratio in [0, 1]:
//
//	rho = clamp(1 - v / vFree, 0, 1)
//
// A vehicle moving at or above free-flow speed gives 0, a stopped vehicle gives
// 1. A non-positive free-flow speed yields 0 instead of dividing by zero.
func Congestion(speedMPS, freeFlowMPS float64) float64 {
	if freeFlowMPS <= 0 {
		return 0
	}
	return clamp01(1 - speedMPS/freeFlowMPS)
}

// clamp01 limits v to the closed interval [0, 1].
func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
