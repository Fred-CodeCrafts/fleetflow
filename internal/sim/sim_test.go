package sim

import (
	"math"
	"testing"

	"fleetflow/internal/graph"
)

// openGraph builds a two-edge graph A->B->C with a capacity so large that
// occupancy barely affects speed, which keeps the expected timings exact.
func openGraph(t *testing.T) *graph.Graph {
	t.Helper()

	g := graph.New()
	for _, id := range []string{"A", "B", "C"} {
		if err := g.AddNode(graph.Node{ID: id}); err != nil {
			t.Fatalf("unexpected error adding node %s: %v", id, err)
		}
	}
	edges := []graph.Edge{
		{ID: "e_AB", From: "A", To: "B", LengthM: 100, SpeedLimitMPS: 10, Lanes: 1},
		{ID: "e_BC", From: "B", To: "C", LengthM: 50, SpeedLimitMPS: 10, Lanes: 1},
	}
	for _, e := range edges {
		if err := g.AddEdge(e); err != nil {
			t.Fatalf("unexpected error adding edge %s: %v", e.ID, err)
		}
	}
	// Make capacity huge so (n/capacity)^4 is negligible and speed is free flow.
	for i := range g.Edges {
		g.Edges[i].Capacity = 1e12
	}
	return g
}

// newTestSim builds a simulation with a single vehicle, which the tests then
// place on a route by hand.
func newTestSim(t *testing.T, g *graph.Graph, stepS float64) *Sim {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Vehicles = 1
	cfg.StepS = stepS

	s, err := New(g, cfg)
	if err != nil {
		t.Fatalf("unexpected error creating simulation: %v", err)
	}
	if len(s.Vehicles()) != 1 {
		t.Fatalf("expected one vehicle, got %d", len(s.Vehicles()))
	}
	return s
}

// placeSolo puts the simulation's only vehicle on the given route at time zero
// and clears the seeded departure time, so timings are exact.
func placeSolo(t *testing.T, s *Sim, route []int) *Vehicle {
	t.Helper()

	v := s.Vehicles()[0]
	v.departS = 0
	s.placeOnRoute(v, route, 0)
	return v
}

func TestStep_ArrivesAfterTheEdgeIsCovered(t *testing.T) {
	g := openGraph(t)
	s := newTestSim(t, g, 1)
	v := placeSolo(t, s, []int{0})

	// e_AB is 100 m at 10 m/s: ten one-second steps.
	for i := 0; i < 9; i++ {
		s.Step()
		if v.State() != VehicleRunning {
			t.Fatalf("vehicle arrived early, after step %d", i+1)
		}
	}
	if got := s.Occupancy().Count(0); got != 1 {
		t.Errorf("occupancy during travel = %d, want 1", got)
	}

	s.Step()

	if v.State() != VehicleArrived {
		t.Fatalf("State() = %s after ten steps, want arrived", v.State())
	}
	if got, ok := v.TripDuration(); !ok || math.Abs(got-10) > 1e-9 {
		t.Errorf("TripDuration() = %f (ok=%v), want 10", got, ok)
	}
	if got := s.Occupancy().Count(0); got != 0 {
		t.Errorf("occupancy after arrival = %d, want 0", got)
	}
	if got := s.Occupancy().Total(); got != 0 {
		t.Errorf("total occupancy after arrival = %d, want 0", got)
	}
	if got := s.CompletedTrips(); len(got) != 1 || math.Abs(got[0]-10) > 1e-9 {
		t.Errorf("CompletedTrips() = %v, want [10]", got)
	}
	if got := s.NowS(); got != 10 {
		t.Errorf("NowS() = %f, want 10", got)
	}
}

func TestStep_CarriesLeftoverTimeOntoTheNextEdge(t *testing.T) {
	g := openGraph(t)
	// One 14-second step covers the 10 s first edge and 4 s of the 5 s second.
	s := newTestSim(t, g, 14)
	v := placeSolo(t, s, []int{0, 1})

	s.Step()

	if got := v.currentEdge(); got != 1 {
		t.Fatalf("currentEdge() = %d, want 1", got)
	}
	// e_BC is 50 m and 4 of its 5 seconds are done.
	if got, want := v.Progress(g.Edges[1].LengthM), 0.8; math.Abs(got-want) > 1e-9 {
		t.Errorf("Progress() = %f, want %f", got, want)
	}
	if got := s.Occupancy().Count(0); got != 0 {
		t.Errorf("occupancy on the first edge = %d, want 0", got)
	}
	if got := s.Occupancy().Count(1); got != 1 {
		t.Errorf("occupancy on the second edge = %d, want 1", got)
	}

	s.Step()

	if v.State() != VehicleArrived {
		t.Fatalf("State() = %s, want arrived", v.State())
	}
	if got, _ := v.TripDuration(); math.Abs(got-15) > 1e-9 {
		t.Errorf("TripDuration() = %f, want 15", got)
	}
}

func TestPlaceOnRoute_EmptyRouteArrivesImmediately(t *testing.T) {
	g := openGraph(t)
	s := newTestSim(t, g, 1)
	v := placeSolo(t, s, nil)

	if v.State() != VehicleArrived {
		t.Errorf("State() = %s, want arrived", v.State())
	}
	if got, ok := v.TripDuration(); !ok || got != 0 {
		t.Errorf("TripDuration() = %f (ok=%v), want 0", got, ok)
	}
	if got := s.Occupancy().Total(); got != 0 {
		t.Errorf("total occupancy = %d, want 0", got)
	}
}

func TestStep_StuckVehicleStaysPut(t *testing.T) {
	g := openGraph(t)
	// A zero free-flow speed on the first edge means nobody can move.
	g.Edges[0].SpeedLimitMPS = 0
	s := newTestSim(t, g, 1)
	v := placeSolo(t, s, []int{0})

	s.Step()

	if v.State() != VehicleRunning {
		t.Errorf("State() = %s, want running", v.State())
	}
	if got := v.Progress(g.Edges[0].LengthM); got != 0 {
		t.Errorf("Progress() = %f, want 0", got)
	}
}
