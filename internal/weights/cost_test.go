package weights

import "testing"

func TestCost(t *testing.T) {
	tests := []struct {
		name      string
		freeFlowS float64
		alpha     float64
		beta      float64
		rhoBar    float64
		want      float64
	}{
		{name: "free-flow has no penalty", freeFlowS: 20, alpha: 1.5, beta: 2, rhoBar: 0, want: 20},
		{name: "fully congested adds alpha", freeFlowS: 20, alpha: 1.5, beta: 2, rhoBar: 1, want: 50},
		{name: "half congested is quadratic", freeFlowS: 40, alpha: 1.5, beta: 2, rhoBar: 0.5, want: 55},
		{name: "negative rho clamps to free-flow", freeFlowS: 20, alpha: 1.5, beta: 2, rhoBar: -1, want: 20},
		{name: "rho above one clamps", freeFlowS: 20, alpha: 1.5, beta: 2, rhoBar: 2, want: 50},
		{name: "negative alpha counts as zero", freeFlowS: 20, alpha: -3, beta: 2, rhoBar: 1, want: 20},
		{name: "non-positive beta falls back to linear", freeFlowS: 20, alpha: 1, beta: 0, rhoBar: 0.5, want: 30},
		{name: "zero free-flow time", freeFlowS: 0, alpha: 1.5, beta: 2, rhoBar: 1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Cost(tt.freeFlowS, tt.alpha, tt.beta, tt.rhoBar)
			if !approxEqual(got, tt.want) {
				t.Errorf("Cost(%f, %f, %f, %f) = %f, want %f",
					tt.freeFlowS, tt.alpha, tt.beta, tt.rhoBar, got, tt.want)
			}
		})
	}
}

func TestCost_GrowsWithCongestion(t *testing.T) {
	const freeFlowS, alpha, beta = 30, 1.5, 2

	previous := Cost(freeFlowS, alpha, beta, 0)
	for rho := 0.1; rho <= 1.0; rho += 0.1 {
		got := Cost(freeFlowS, alpha, beta, rho)
		if got < previous {
			t.Fatalf("cost fell from %f to %f at rho %f", previous, got, rho)
		}
		previous = got
	}
}
