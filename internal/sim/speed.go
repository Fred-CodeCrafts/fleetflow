package sim

import "math"

// bprCoefficient and bprExponent shape the volume-delay curve reused to turn
// occupancy into speed. The fourth power keeps an edge near free flow until it
// is nearly full, then slows it sharply.
const (
	bprCoefficient = 0.15
	bprExponent    = 4
)

// EdgeSpeed returns the speed vehicles achieve on an edge carrying n vehicles:
//
//	v = vFree / (1 + 0.15 * (n / capacity)^4)
//
// This is the classic BPR volume-delay shape, used here with vehicle count
// instead of traffic volume. A non-positive capacity or vehicle count falls back
// to free-flow speed rather than dividing by zero.
func EdgeSpeed(vFreeMPS, capacity float64, n int) float64 {
	if vFreeMPS <= 0 {
		return 0
	}
	if capacity <= 0 || n <= 0 {
		return vFreeMPS
	}
	ratio := float64(n) / capacity
	return vFreeMPS / (1 + bprCoefficient*math.Pow(ratio, bprExponent))
}
