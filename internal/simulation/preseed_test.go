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

// A simulated trip has to name the vehicle that drove it and the places it ran
// between: without them the console can only show an unassigned trip on an
// unnamed route, and a scenario vehicle can never be reconciled with a fleet
// record.
func TestSimulatedTripsCarryVehicleAndRealPlaces(t *testing.T) {
	res := simulation.GenerateSyntheticScenario(7, 3, 300)
	if len(res.Trips) == 0 {
		t.Fatal("generated scenario has no trips")
	}
	named := make(map[string]bool, len(simulation.JakartaHubs))
	for _, hub := range simulation.JakartaHubs {
		named[hub.Name] = true
	}
	for _, trip := range res.Trips {
		if trip.VehicleID == "" {
			t.Fatalf("trip %q has no vehicle", trip.ID)
		}
		if !named[trip.Origin] || !named[trip.Destination] {
			t.Fatalf("trip %q runs %q -> %q, which are not Jakarta hubs", trip.ID, trip.Origin, trip.Destination)
		}
		if trip.Origin == trip.Destination {
			t.Fatalf("trip %q never leaves its origin", trip.ID)
		}
	}

	preseeded := simulation.DefaultPreseededRun()
	for _, trip := range preseeded.Result.Trips {
		if trip.VehicleID == "" {
			t.Fatalf("preseeded trip %q has no vehicle", trip.ID)
		}
		if !named[trip.Origin] || !named[trip.Destination] {
			t.Fatalf("preseeded trip %q runs %q -> %q, which are not Jakarta hubs", trip.ID, trip.Origin, trip.Destination)
		}
	}
	// The run's own tracks must answer to the vehicles its trips name, or the
	// console cannot draw the route of a trip it lists.
	for _, trip := range preseeded.Result.Trips {
		found := false
		for _, track := range preseeded.Result.Tracks {
			if track.VehicleID == trip.VehicleID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("preseeded trip %q names vehicle %q with no position track", trip.ID, trip.VehicleID)
		}
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
