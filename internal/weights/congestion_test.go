package weights

import "testing"

func TestCongestion(t *testing.T) {
	tests := []struct {
		name           string
		speedMPS       float64
		freeFlowMPS    float64
		wantCongestion float64
	}{
		{name: "at free-flow speed", speedMPS: 13.89, freeFlowMPS: 13.89, wantCongestion: 0},
		{name: "half free-flow speed", speedMPS: 7.5, freeFlowMPS: 15, wantCongestion: 0.5},
		{name: "stopped", speedMPS: 0, freeFlowMPS: 13.89, wantCongestion: 1},
		{name: "faster than free-flow clamps to zero", speedMPS: 20, freeFlowMPS: 15, wantCongestion: 0},
		{name: "negative speed clamps to one", speedMPS: -5, freeFlowMPS: 15, wantCongestion: 1},
		{name: "zero free-flow speed", speedMPS: 5, freeFlowMPS: 0, wantCongestion: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Congestion(tt.speedMPS, tt.freeFlowMPS)
			if got != tt.wantCongestion {
				t.Errorf("Congestion(%f, %f) = %f, want %f",
					tt.speedMPS, tt.freeFlowMPS, got, tt.wantCongestion)
			}
		})
	}
}
