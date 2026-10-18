package sim

import (
	"testing"
	"time"

	"fleetflow/internal/engine"
	"fleetflow/internal/weights"
)

// TestStep_RefreshesWeightsOnTheSimulationClock checks that a real engine has its
// weights rebuilt as simulated time passes, not only when the run starts.
func TestStep_RefreshesWeightsOnTheSimulationClock(t *testing.T) {
	g := loadFixture(t)

	params := weights.DefaultParams()
	params.Delta = 5 * time.Second // refresh every five simulated seconds

	e, err := engine.New(g, engine.NewDijkstraRouter(g), params)
	if err != nil {
		t.Fatalf("unexpected error creating engine: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Vehicles = 1
	cfg.FleetShare = 1
	cfg.StepS = 1
	cfg.WindowS = 10
	s, err := New(g, cfg)
	if err != nil {
		t.Fatalf("unexpected error creating simulation: %v", err)
	}
	s.SetFleetService(NewEngineFleet(e))

	if got := e.ActiveMetric().Version(); got != 1 {
		t.Fatalf("initial version = %d, want 1", got)
	}

	// A few steps is not yet a customization interval.
	for i := 0; i < 4; i++ {
		s.Step()
	}
	if got := e.ActiveMetric().Version(); got != 1 {
		t.Errorf("version = %d after 4 s, want no refresh yet", got)
	}

	s.Step() // the fifth second triggers a refresh
	if got := e.ActiveMetric().Version(); got != 2 {
		t.Errorf("version = %d after 5 s, want 2", got)
	}

	for i := 0; i < 5; i++ {
		s.Step()
	}
	if got := e.ActiveMetric().Version(); got != 3 {
		t.Errorf("version = %d after 10 s, want 3", got)
	}
}

func TestSetFleetService_IgnoresPlainServices(t *testing.T) {
	s, g := newFixtureSim(t, 1, 1)

	// A fake service does not refresh weights, so the simulation must not try.
	s.SetFleetService(newFakeFleet(len(g.Edges)))
	if s.customizer != nil {
		t.Error("expected no customizer for a plain fleet service")
	}

	s.Step() // must not panic
}
