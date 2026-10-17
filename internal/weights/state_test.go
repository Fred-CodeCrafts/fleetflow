package weights

import "testing"

func TestNewEdgeState_StartsAtFreeFlow(t *testing.T) {
	s := NewEdgeState()

	if got := s.Rho(); got != 0 {
		t.Errorf("expected Rho() = 0 for a fresh edge, got %f", got)
	}
}
