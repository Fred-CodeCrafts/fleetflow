package sim

import (
	"testing"
	"time"

	"fleetflow/internal/weights"
)

func comparisonConfig() Config {
	cfg := DefaultConfig()
	cfg.Seed = 2026
	cfg.Vehicles = 60
	cfg.FleetShare = 0.5
	cfg.WindowS = 120
	cfg.StepS = 1
	cfg.RerouteEveryS = 15
	cfg.RerouteThreshold = 0.10
	return cfg
}

func TestRunBoth_BothRunsFinishOnTheSameDemand(t *testing.T) {
	g := loadFixture(t)

	params := weights.DefaultParams()
	params.Delta = 5 * time.Second

	comparison, err := RunBoth(g, comparisonConfig(), params, 20000)
	if err != nil {
		t.Fatalf("unexpected error running the comparison: %v", err)
	}

	for name, m := range map[string]Metrics{
		"baseline":  comparison.Baseline,
		"fleetflow": comparison.FleetFlow,
	} {
		if m.TotalVehicles != 60 {
			t.Errorf("%s: TotalVehicles = %d, want 60", name, m.TotalVehicles)
		}
		if m.CompletedTrips != m.TotalVehicles {
			t.Errorf("%s: completed %d of %d trips", name, m.CompletedTrips, m.TotalVehicles)
		}
		if m.AverageTripS <= 0 {
			t.Errorf("%s: AverageTripS = %f, want a positive time", name, m.AverageTripS)
		}
		if m.P95TripS < m.AverageTripS {
			t.Errorf("%s: P95TripS %f is below the average %f", name, m.P95TripS, m.AverageTripS)
		}
		if m.UnroutableVehicles != 0 {
			t.Errorf("%s: %d unroutable vehicles on a connected graph", name, m.UnroutableVehicles)
		}
	}
}

func TestRunBoth_IsDeterministic(t *testing.T) {
	g := loadFixture(t)
	params := weights.DefaultParams()
	params.Delta = 5 * time.Second

	first, err := RunBoth(g, comparisonConfig(), params, 20000)
	if err != nil {
		t.Fatalf("unexpected error running the first comparison: %v", err)
	}
	second, err := RunBoth(g, comparisonConfig(), params, 20000)
	if err != nil {
		t.Fatalf("unexpected error running the second comparison: %v", err)
	}

	if first != second {
		t.Errorf("the same seed produced different results:\nfirst  = %+v\nsecond = %+v", first, second)
	}
}

func TestRunToCompletion_StopsAtTheStepBudget(t *testing.T) {
	g := loadFixture(t)
	cfg := comparisonConfig()
	cfg.WindowS = 100000 // nobody departs within the budget

	s, err := New(g, cfg)
	if err != nil {
		t.Fatalf("unexpected error creating simulation: %v", err)
	}

	m := s.RunToCompletion(5)

	if m.CompletedTrips != 0 {
		t.Errorf("CompletedTrips = %d, want 0", m.CompletedTrips)
	}
	if m.TotalVehicles != cfg.Vehicles {
		t.Errorf("TotalVehicles = %d, want %d", m.TotalVehicles, cfg.Vehicles)
	}
	if got := s.NowS(); got != 5 {
		t.Errorf("NowS() = %f after a five-step budget, want 5", got)
	}
}
