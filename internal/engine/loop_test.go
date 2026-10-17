package engine

import (
	"context"
	"testing"
	"time"

	"fleetflow/internal/weights"
)

// waitFor polls cond until it is true or the deadline passes.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before the deadline")
}

func TestRun_CustomizesPeriodicallyAndStopsCleanly(t *testing.T) {
	g := loadFixture(t)
	p := weights.DefaultParams()
	p.Delta = 5 * time.Millisecond

	e, err := New(g, NewDijkstraRouter(g), p)
	if err != nil {
		t.Fatalf("unexpected error creating engine: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.Run(ctx)
		close(done)
	}()

	waitFor(t, 3*time.Second, func() bool { return e.ActiveMetric().Version() > 1 })

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}

	// After shutdown the version must stop moving.
	stopped := e.ActiveMetric().Version()
	time.Sleep(30 * time.Millisecond)
	if got := e.ActiveMetric().Version(); got != stopped {
		t.Errorf("version advanced from %d to %d after shutdown", stopped, got)
	}
}

func TestDecayStale_RelaxesQuietEdges(t *testing.T) {
	g := loadFixture(t)
	p := weights.DefaultParams()
	e, err := New(g, NewDijkstraRouter(g), p)
	if err != nil {
		t.Fatalf("unexpected error creating engine: %v", err)
	}

	t0 := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	idx := edgeIndex(t, g, "e_AB")
	if err := e.Report("e_AB", 0, t0); err != nil { // rho 0.3 after one report
		t.Fatalf("unexpected report error: %v", err)
	}
	congested := e.Customize().Cost(idx)

	// Within the hold time nothing relaxes.
	e.DecayStale(t0.Add(p.Hold - time.Second))
	e.Customize()
	if got := e.ActiveMetric().Cost(idx); got != congested {
		t.Errorf("cost = %f, want no change (%f) within the hold time", got, congested)
	}

	// Past the hold time the edge relaxes towards free flow.
	e.DecayStale(t0.Add(p.Hold + time.Second))
	e.Customize()
	if got := e.ActiveMetric().Cost(idx); !(got < congested) {
		t.Errorf("cost = %f, want less than %f after decay", got, congested)
	}
}
