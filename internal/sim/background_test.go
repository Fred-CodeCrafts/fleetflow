package sim

import (
	"math"
	"testing"

	"fleetflow/internal/graph"
)

// newFixtureSim builds a simulation on the shared four-node fixture graph.
func newFixtureSim(t *testing.T, stepS float64, fleetShare float64) (*Sim, *graph.Graph) {
	t.Helper()

	g := loadFixture(t)
	cfg := DefaultConfig()
	cfg.Vehicles = 1
	cfg.FleetShare = fleetShare
	cfg.StepS = stepS
	cfg.WindowS = 10

	s, err := New(g, cfg)
	if err != nil {
		t.Fatalf("unexpected error creating simulation: %v", err)
	}
	return s, g
}

// soloTrip fixes the simulation's only vehicle to a specific trip.
func soloTrip(t *testing.T, s *Sim, from, to string, fleet bool) *Vehicle {
	t.Helper()

	if len(s.Vehicles()) != 1 {
		t.Fatalf("expected one vehicle, got %d", len(s.Vehicles()))
	}
	v := s.Vehicles()[0]
	v.demand.From = from
	v.demand.To = to
	v.demand.Fleet = fleet
	return v
}

func TestStep_BackgroundVehicleTakesTheStaticRoute(t *testing.T) {
	s, g := newFixtureSim(t, 1, 0) // no fleet vehicles at all
	v := soloTrip(t, s, "A", "D", false)
	v.departS = 0

	s.Step()

	if v.State() != VehicleRunning {
		t.Fatalf("State() = %s, want running after departure", v.State())
	}

	want := []int{edgeIndexByID(t, g, "e_AC"), edgeIndexByID(t, g, "e_CD")}
	if len(v.route) != len(want) || v.route[0] != want[0] || v.route[1] != want[1] {
		t.Fatalf("route = %v, want %v", v.route, want)
	}

	// The vehicle should finish close to the free-flow time of that route.
	var freeFlow float64
	for _, idx := range want {
		freeFlow += g.Edges[idx].FreeFlowS
	}
	for i := 0; i < 500 && v.State() == VehicleRunning; i++ {
		s.Step()
	}
	if v.State() != VehicleArrived {
		t.Fatalf("State() = %s, want arrived", v.State())
	}
	if got, _ := v.TripDuration(); math.Abs(got-freeFlow) > 2 {
		t.Errorf("TripDuration() = %f, want about %f", got, freeFlow)
	}
}

func TestStep_UnreachableDestinationIsUnroutable(t *testing.T) {
	// openGraph is a one-way chain A->B->C, so C cannot reach A.
	g := openGraph(t)
	cfg := DefaultConfig()
	cfg.Vehicles = 1
	cfg.WindowS = 10
	s, err := New(g, cfg)
	if err != nil {
		t.Fatalf("unexpected error creating simulation: %v", err)
	}

	v := soloTrip(t, s, "C", "A", false)
	v.departS = 0

	s.Step()

	if v.State() != VehicleUnroutable {
		t.Errorf("State() = %s, want unroutable", v.State())
	}
	if got := s.Occupancy().Total(); got != 0 {
		t.Errorf("total occupancy = %d, want 0 for an unroutable vehicle", got)
	}
}

// edgeIndexByID returns the index of the edge with the given ID.
func edgeIndexByID(t *testing.T, g *graph.Graph, id string) int {
	t.Helper()
	_, idx, ok := g.EdgeByID(id)
	if !ok {
		t.Fatalf("edge %s not found", id)
	}
	return idx
}
