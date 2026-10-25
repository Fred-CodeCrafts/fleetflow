package engine

import "testing"

func TestMetric_ExposesCostsAndVersion(t *testing.T) {
	costs := []float64{10, 20.5, 30}
	m := NewMetric(costs, 7)

	if got := m.Version(); got != 7 {
		t.Errorf("Version() = %d, want 7", got)
	}
	if got := m.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}

	for i, want := range costs {
		if got := m.Cost(i); got != want {
			t.Errorf("Cost(%d) = %f, want %f", i, got, want)
		}
	}
}

func TestMetric_SnapshotsAreIndependent(t *testing.T) {
	// The customizer builds a fresh cost slice per cycle, so an older snapshot
	// must not change when a newer one is built.
	first := NewMetric([]float64{1, 2}, 1)
	second := NewMetric([]float64{9, 8}, 2)

	if first.Cost(0) != 1 || first.Cost(1) != 2 {
		t.Errorf("first snapshot changed: %f, %f", first.Cost(0), first.Cost(1))
	}
	if second.Cost(0) != 9 || second.Cost(1) != 8 {
		t.Errorf("second snapshot wrong: %f, %f", second.Cost(0), second.Cost(1))
	}
	if first.Version() == second.Version() {
		t.Error("expected distinct versions for two snapshots")
	}
}
