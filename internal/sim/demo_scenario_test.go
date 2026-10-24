package sim

import (
	"testing"

	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
)

// TestDemoScenarioMakesTheArmsDiffer pins the property the demo defaults exist to
// produce, because losing it is silent.
//
// A comparison whose two arms behave identically fails nothing. The dashboard
// draws the same picture twice, every metric ties, and the result reads as
// "dynamic routing changes nothing" -- which is what happened while this network
// was 450 m blocks with two and three lanes: its capacity is far beyond any fleet
// this simulator can drive, so no edge ever reached the occupancy the speed model
// needs before it slows anything down. Changing the network or the default
// departure window can bring that back, and this test fails instead of leaving it
// to whoever notices the demo has no numbers in it.
//
// DESIGN 3.1 and docs/results.md carry the measurement; this keeps it true.
func TestDemoScenarioMakesTheArmsDiffer(t *testing.T) {
	g, err := graph.LoadFromFile("../../data/graph.json")
	if err != nil {
		t.Fatalf("load the demo graph: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Seed = 1

	comparison, err := RunBoth(g, cfg, weights.DefaultParams(), 200_000)
	if err != nil {
		t.Fatalf("RunBoth: %v", err)
	}

	baseline, fleetflow := comparison.Baseline, comparison.FleetFlow

	// Both arms complete the demand: the difference is what it cost, not whether
	// the vehicles arrived.
	if baseline.CompletedTrips != cfg.Vehicles || fleetflow.CompletedTrips != cfg.Vehicles {
		t.Fatalf("completed trips = %d baseline, %d fleetflow, want %d each",
			baseline.CompletedTrips, fleetflow.CompletedTrips, cfg.Vehicles)
	}

	// The baseline asks for new routes and always declines, because its metric
	// never moves. Its zero is by construction; pinning it keeps that clear
	// rather than looking like an artefact of this seed.
	if baseline.RouteChanges != 0 {
		t.Errorf("baseline route changes = %d, want 0: its weights never move", baseline.RouteChanges)
	}

	// If the dynamic arm never reroutes either, the comparison is between two
	// static arms wearing different names.
	if fleetflow.RouteChanges == 0 {
		t.Fatal("fleetflow route changes = 0: this scenario gives the dynamic router nothing to do")
	}

	// And the rerouting has to pay, by a margin that measurement noise cannot
	// invent. On seed 1 the baseline averages about 83 s a trip and FleetFlow
	// about 44, so this is a wide band rather than a tight one.
	if fleetflow.AverageTripS >= baseline.AverageTripS {
		t.Errorf("fleetflow average trip = %.1fs, baseline = %.1fs: the dynamic arm must be faster on the demo scenario",
			fleetflow.AverageTripS, baseline.AverageTripS)
	}
}
