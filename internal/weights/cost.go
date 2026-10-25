package weights

import "math"

// Cost is the routing weight of an edge, in seconds:
//
//	cost = freeFlowS * (1 + alpha * rhoBar^beta)
//
// This follows the BPR shape (free-flow time multiplied by a penalty that grows
// non-linearly with congestion), but the penalty is driven by the smoothed
// observed congestion instead of volume over capacity.
//
// alpha is the penalty strength and beta how sharply the penalty rises. Inputs
// are clamped so the result is always finite and never below free-flow time:
// rhoBar is limited to [0, 1], a negative alpha counts as 0, and a non-positive
// beta falls back to a linear penalty (exponent 1).
func Cost(freeFlowS, alpha, beta, rhoBar float64) float64 {
	if freeFlowS <= 0 {
		return 0
	}

	rho := clamp01(rhoBar)
	a := math.Max(alpha, 0)
	b := beta
	if b <= 0 {
		b = 1
	}

	return freeFlowS * (1 + a*math.Pow(rho, b))
}
