package sim

import (
	"math"
	"testing"
)

// runUntilArrived steps the simulation until the vehicle arrives or the step
// budget runs out, so tests do not have to repeat the loop condition.
func runUntilArrived(t *testing.T, s *Sim, v *Vehicle) {
	t.Helper()

	for i := 0; i < 5000 && v.State() == VehicleRunning; i++ {
		s.Step()
	}
	if v.State() != VehicleArrived {
		t.Fatalf("State() = %s, want arrived", v.State())
	}
}

func TestCollect_TimeLostIsZeroAtFreeFlow(t *testing.T) {
	g := openGraph(t)
	s := newTestSim(t, g, 1)
	v := placeSolo(t, s, []int{0, 1}) // 100 m + 50 m at 10 m/s: 15 s of free flow

	runUntilArrived(t, s, v)

	m := s.Collect()
	if m.Fairness.MeanTimeLostS != 0 || m.Fairness.TotalTimeLostS != 0 {
		t.Errorf("time lost at free flow = %f mean, %f total, want 0 and 0",
			m.Fairness.MeanTimeLostS, m.Fairness.TotalTimeLostS)
	}
}

func TestCollect_TimeLostIsPositiveOnASlowerTrip(t *testing.T) {
	g := openGraph(t)
	g.Edges[0].Capacity = 1 // the vehicle's own presence now slows it below free flow
	s := newTestSim(t, g, 1)
	v := placeSolo(t, s, []int{0})

	runUntilArrived(t, s, v)

	m := s.Collect()
	// 100 m at 10 m/s is 10 s of free flow, and at 10/(1 + 0.15) m/s it takes
	// 11.5 s, so the trip should have lost about 1.5 s.
	if m.Fairness.TotalTimeLostS <= 0 {
		t.Fatalf("TotalTimeLostS = %f, want a positive loss on a slower trip", m.Fairness.TotalTimeLostS)
	}
	if got := m.Fairness.MeanTimeLostS; math.Abs(got-1.5) > 0.05 {
		t.Errorf("MeanTimeLostS = %f, want about 1.5", got)
	}
}

func TestCollect_SplitsFleetAndBackgroundTripTimes(t *testing.T) {
	g := openGraph(t)
	cfg := DefaultConfig()
	cfg.Vehicles = 2
	cfg.StepS = 1
	s, err := New(g, cfg)
	if err != nil {
		t.Fatalf("unexpected error creating simulation: %v", err)
	}
	if len(s.Vehicles()) != 2 {
		t.Fatalf("expected two vehicles, got %d", len(s.Vehicles()))
	}

	// The fleet vehicle drives the 100 m first edge, the background vehicle the
	// 50 m second one, both at free flow.
	fleetVehicle := s.Vehicles()[0]
	fleetVehicle.demand.Fleet = true
	fleetVehicle.departS = 0
	s.placeOnRoute(fleetVehicle, []int{0}, 0)

	backgroundVehicle := s.Vehicles()[1]
	backgroundVehicle.demand.Fleet = false
	backgroundVehicle.departS = 0
	s.placeOnRoute(backgroundVehicle, []int{1}, 0)

	for i := 0; i < 5000 && (fleetVehicle.State() == VehicleRunning || backgroundVehicle.State() == VehicleRunning); i++ {
		s.Step()
	}

	m := s.Collect()
	if m.Fairness.FleetTrips != 1 {
		t.Fatalf("FleetTrips = %d, want 1", m.Fairness.FleetTrips)
	}
	if m.Fairness.BackgroundTrips != 1 {
		t.Fatalf("BackgroundTrips = %d, want 1", m.Fairness.BackgroundTrips)
	}
	if got := m.Fairness.MeanFleetTripS; math.Abs(got-10) > 1e-9 {
		t.Errorf("MeanFleetTripS = %f, want 10", got)
	}
	if got := m.Fairness.MeanBackgroundTripS; math.Abs(got-5) > 1e-9 {
		t.Errorf("MeanBackgroundTripS = %f, want 5", got)
	}
}

func TestCollect_FairnessBaselineFollowsTheDrivenRoute(t *testing.T) {
	// The vehicle is routed A->D via e_AC + e_CD, then switches to the free
	// alternative e_CB + e_BD before it reaches the dropped edge. The baseline
	// must be the three edges it actually drove, never the theoretical route.
	s, v, fleet, g, edges := rerouteFixture(t, 0.10)
	fleet.costs[edges[2]] = 0
	fleet.costs[edges[3]] = 0

	for i := 0; i < 11; i++ {
		s.Step()
	}
	if v.RouteChanges() != 1 {
		t.Fatalf("RouteChanges() = %d, want 1", v.RouteChanges())
	}

	runUntilArrived(t, s, v)

	want := g.Edges[edges[0]].FreeFlowS + g.Edges[edges[2]].FreeFlowS + g.Edges[edges[3]].FreeFlowS
	if math.Abs(v.freeFlowS-want) > 1e-9 {
		t.Errorf("freeFlowS = %f, want %f for the driven route", v.freeFlowS, want)
	}
}

func TestCollect_QuietRunFairnessIsZeroed(t *testing.T) {
	g := openGraph(t)
	s := newTestSim(t, g, 1)

	// Never place the vehicle, so nothing departs and nothing completes.
	m := s.Collect()

	f := m.Fairness
	if f.FleetTrips != 0 || f.BackgroundTrips != 0 {
		t.Errorf("trip counts = %d fleet, %d background, want 0 and 0", f.FleetTrips, f.BackgroundTrips)
	}
	for name, got := range map[string]float64{
		"MeanTimeLostS":       f.MeanTimeLostS,
		"TotalTimeLostS":      f.TotalTimeLostS,
		"MeanFleetTripS":      f.MeanFleetTripS,
		"MeanBackgroundTripS": f.MeanBackgroundTripS,
	} {
		if math.IsNaN(got) {
			t.Errorf("%s = NaN on a quiet run", name)
		}
		if got != 0 {
			t.Errorf("%s = %f on a quiet run, want 0", name, got)
		}
	}
}
