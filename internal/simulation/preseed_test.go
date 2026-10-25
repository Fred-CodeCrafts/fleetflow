package simulation_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"fleetflow/internal/simulation"
	"fleetflow/internal/simulation/memory"
	"fleetflow/internal/simulation/sumo"
)

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
}

func TestSyntheticRunnerUsesRequestedSeed(t *testing.T) {
	runner := simulation.NewSyntheticRunner()
	res, err := runner.Run(context.Background(), sumo.Scenario{Seed: 999, Vehicles: 5})
	if err != nil {
		t.Fatalf("synthetic runner failed: %v", err)
	}
	if res.Seed != 999 {
		t.Fatalf("synthetic runner seed = %d, want 999", res.Seed)
	}
}

// The three presets must be stored as real completed runs whose size matches the
// mode they stand for, and seeding twice must not duplicate them: a restart is
// not a second scenario.
func TestSeedPresetRunsIsIdempotentAndComplete(t *testing.T) {
	repo := memory.New()
	svc, err := simulation.NewService(context.Background(), repo, simulation.NewSyntheticRunner(), nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer func() { _ = svc.Shutdown(context.Background()) }()

	if err := svc.SeedPresetRuns(context.Background()); err != nil {
		t.Fatalf("SeedPresetRuns failed: %v", err)
	}

	for _, preset := range simulation.PresetRuns {
		seeded, err := svc.Get(context.Background(), preset.ID)
		if err != nil {
			t.Fatalf("Get preset %q failed: %v", preset.ID, err)
		}
		if seeded.Status != simulation.StatusCompleted {
			t.Fatalf("preset %q status = %q, want completed", preset.ID, seeded.Status)
		}
		if seeded.Result == nil {
			t.Fatalf("preset %q has no result", preset.ID)
		}
		if got := len(seeded.Result.Trips); got != preset.Scenario.Vehicles {
			t.Fatalf("preset %q has %d trips, want %d", preset.ID, got, preset.Scenario.Vehicles)
		}
		if len(seeded.Result.Tracks) == 0 || len(seeded.Result.Deliveries) == 0 {
			t.Fatalf("preset %q is missing tracks or deliveries", preset.ID)
		}
	}

	if err := svc.SeedPresetRuns(context.Background()); err != nil {
		t.Fatalf("second SeedPresetRuns failed: %v", err)
	}
	runs, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(runs) != len(simulation.PresetRuns) {
		t.Fatalf("seeding twice produced %d runs, want %d", len(runs), len(simulation.PresetRuns))
	}
}

// The fleet register is only as large as the widest preset: all three modes draw
// from one plate sequence, so a peak run's vehicles are the first fifty of the
// same fleet rather than a second one.
// The dispatcher deduplicates a trip on its vehicle plus the two hubs, so two
// preset modes must never produce that key for the same vehicle: if they did, the
// second mode's trip would be silently skipped as a duplicate. This test is what
// keeps the reuse a decision rather than a lucky collision.
func TestPresetTripsDoNotCollide(t *testing.T) {
	seen := map[string]string{}
	for _, preset := range simulation.PresetRuns {
		res := simulation.GenerateSyntheticScenario(preset.Scenario.Seed, preset.Scenario.Vehicles, preset.Scenario.DepartureWindowS)
		for _, trip := range res.Trips {
			key := strings.ToLower(trip.VehicleID) + "|" +
				strings.ToLower(strings.TrimSpace(trip.Origin)) + "|" +
				strings.ToLower(strings.TrimSpace(trip.Destination))
			if other, ok := seen[key]; ok {
				t.Fatalf("presets %q and %q both route %s from %s to %s; one of those trips would be dropped as a duplicate",
					other, preset.ID, trip.VehicleID, trip.Origin, trip.Destination)
			}
			seen[key] = preset.ID
		}
	}
}

func TestPresetsShareOneFleetSequence(t *testing.T) {
	seen := map[string]bool{}
	for _, preset := range simulation.PresetRuns {
		res := simulation.GenerateSyntheticScenario(preset.Scenario.Seed, preset.Scenario.Vehicles, preset.Scenario.DepartureWindowS)
		for i, trip := range res.Trips {
			want := fmt.Sprintf("B-%04d-JKT", 9001+i)
			if trip.VehicleID != want {
				t.Fatalf("preset %q trip %d rides %q, want %q", preset.ID, i, trip.VehicleID, want)
			}
			seen[trip.VehicleID] = true
		}
	}
	if len(seen) > 100 {
		t.Fatalf("presets span %d vehicles, want at most the 100 of the largest mode", len(seen))
	}
}
