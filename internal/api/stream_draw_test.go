package api

import (
	"testing"

	"fleetflow/internal/sim"
)

// bigRun builds a run whose fleet is large enough to exceed what one frame will
// draw, over the repository's test graph.
func bigRun(t *testing.T, vehicles int) *run {
	t.Helper()

	g := testGraph(t)
	cfg := sim.DefaultConfig()
	cfg.Seed = 1
	cfg.Vehicles = vehicles

	baseline, err := sim.New(g, cfg)
	if err != nil {
		t.Fatalf("build the baseline arm: %v", err)
	}
	fleetflow, err := sim.New(g, cfg)
	if err != nil {
		t.Fatalf("build the fleetflow arm: %v", err)
	}

	r := &run{cfg: cfg, g: g, baseline: baseline, fleetflow: fleetflow}
	// Step so the vehicles have departed and are therefore drawable; a pending
	// vehicle is drawn too, but a run of this size starts them all at once.
	r.baseline.Step()
	return r
}

// TestArmVehicles_CapsWhatItDraws pins the property that keeps a large fleet from
// flooding the browser.
//
// The metrics stay exact Ã¢â‚¬â€ they are counted from the simulation, not from this
// frame Ã¢â‚¬â€ so the only thing lost is markers the map could not resolve anyway.
// Uncapped, a 60,000-vehicle run puts both arms' vehicles on the wire five times
// a second: measured at 19.3 MB a frame, 96.6 MB/s.
func TestArmVehicles_CapsWhatItDraws(t *testing.T) {
	r := bigRun(t, 2*maxDrawnVehicles+500)

	// Put every vehicle on the road, so nothing is dropped for being finished.
	r.baseline.Step()

	got := r.armVehicles(armBaseline, r.baseline)
	if len(got) > maxDrawnVehicles {
		t.Errorf("drew %d vehicles, want at most %d", len(got), maxDrawnVehicles)
	}
	if len(got) == 0 {
		t.Fatal("drew no vehicles at all")
	}

	// A frame carries both arms, so the total is bounded by twice the cap.
	all := r.vehicles()
	if len(all) > 2*maxDrawnVehicles {
		t.Errorf("frame carries %d vehicles, want at most %d", len(all), 2*maxDrawnVehicles)
	}
}

// TestArmVehicles_DrawsEverythingBelowTheCap is the other half: sampling must not
// quietly drop vehicles from a small run, which is what every existing test and
// the small-network demo rely on.
func TestArmVehicles_DrawsEverythingBelowTheCap(t *testing.T) {
	const vehicles = 50
	r := bigRun(t, vehicles)
	r.baseline.Step()

	got := r.armVehicles(armBaseline, r.baseline)
	if len(got) != vehicles {
		t.Errorf("drew %d of %d vehicles, want all of them", len(got), vehicles)
	}
}

// TestArmVehicles_PrioritisesFleetVehicles checks the sample is not blind to what
// the run is about: a fleet vehicle is the one whose rerouting the comparison
// measures, so when the frame has to be sampled the fleet gets its own share of
// the budget rather than competing with background traffic for it.
func TestArmVehicles_PrioritisesFleetVehicles(t *testing.T) {
	const vehicles = 2*maxDrawnVehicles + 500
	r := bigRun(t, vehicles)

	fleetOnRoad, backgroundOnRoad := 0, 0
	for _, v := range r.baseline.Vehicles() {
		if v.State() != sim.VehicleRunning && v.State() != sim.VehiclePending {
			continue
		}
		if v.IsFleet() {
			fleetOnRoad++
		} else {
			backgroundOnRoad++
		}
	}

	drawn := r.armVehicles(armBaseline, r.baseline)
	drawnFleet := 0
	for _, v := range drawn {
		if v.Fleet {
			drawnFleet++
		}
	}

	// Half the budget is reserved for the fleet, so the drawn fleet's share must
	// be well above its share of the arm when it is outnumbered.
	if fleetOnRoad < backgroundOnRoad {
		fleetShare := float64(drawnFleet) / float64(len(drawn))
		armShare := float64(fleetOnRoad) / float64(fleetOnRoad+backgroundOnRoad)
		if fleetShare <= armShare {
			t.Errorf("drawn fleet share = %.2f, arm share = %.2f: the sample under-represents the fleet",
				fleetShare, armShare)
		}
	}
	if drawnFleet == 0 {
		t.Error("drew no fleet vehicles at all")
	}
}
