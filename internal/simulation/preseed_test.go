package simulation_test

import (
	"context"
	"testing"

	"fleetflow/internal/simulation"
	"fleetflow/internal/simulation/memory"
	"fleetflow/internal/simulation/sumo"
)

func TestDefaultPreseededRunAndSyntheticRunner(t *testing.T) {
	run := simulation.DefaultPreseededRun()
	if run.ID != simulation.PreseededRunID {
		t.Fatalf("preseeded run ID = %q, want %q", run.ID, simulation.PreseededRunID)
	}
	if run.Result == nil || len(run.Result.Tracks) == 0 || len(run.Result.Trips) == 0 {
		t.Fatalf("preseeded run result is incomplete: %#v", run.Result)
	}
	if len(simulation.JakartaLogisticsHubs) != 5 {
		t.Fatalf("expected 5 logistics hubs, got %d", len(simulation.JakartaLogisticsHubs))
	}

	runner := simulation.NewSyntheticRunner()
	res, err := runner.Run(context.Background(), sumo.Scenario{Seed: 999, Vehicles: 5})
	if err != nil {
		t.Fatalf("synthetic runner failed: %v", err)
	}
	if res.Seed != 999 {
		t.Fatalf("synthetic runner seed = %d, want 999", res.Seed)
	}
}

func TestSeedDefaultRun(t *testing.T) {
	repo := memory.New()
	svc, err := simulation.NewService(context.Background(), repo, simulation.NewSyntheticRunner(), nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer svc.Shutdown(context.Background())

	if err := svc.SeedDefaultRun(context.Background()); err != nil {
		t.Fatalf("SeedDefaultRun failed: %v", err)
	}

	seeded, err := svc.Get(context.Background(), simulation.PreseededRunID)
	if err != nil {
		t.Fatalf("Get preseeded run failed: %v", err)
	}
	if seeded.Status != simulation.StatusCompleted {
		t.Fatalf("seeded status = %q, want completed", seeded.Status)
	}

	// Calling SeedDefaultRun again should be idempotent
	if err := svc.SeedDefaultRun(context.Background()); err != nil {
		t.Fatalf("idempotent SeedDefaultRun failed: %v", err)
	}
}
