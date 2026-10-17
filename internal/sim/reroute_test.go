package sim

import (
	"testing"

	"fleetflow/internal/graph"
)

// rerouteFixture builds a one-vehicle fleet run on the four-node fixture graph,
// with a fake service that offers an alternative route from C to D.
func rerouteFixture(t *testing.T, threshold float64) (*Sim, *Vehicle, *fakeFleet, *graph.Graph, []int) {
	t.Helper()

	g := loadFixture(t)
	s, g2 := newFixtureSim(t, 1, 1)
	v := soloTrip(t, s, "A", "D", true)
	v.departS = 0

	eAC := edgeIndexByID(t, g, "e_AC")
	eCD := edgeIndexByID(t, g, "e_CD")
	eCB := edgeIndexByID(t, g, "e_CB")
	eBD := edgeIndexByID(t, g, "e_BD")

	fleet := newFakeFleet(len(g.Edges))
	fleet.routes["A->D"] = []int{eAC, eCD}
	fleet.routes["C->D"] = []int{eCB, eBD}
	s.SetFleetService(fleet)

	s.cfg.RerouteEveryS = 10
	s.cfg.RerouteThreshold = threshold
	s.cfg.WindowS = 10

	return s, v, fleet, g2, []int{eAC, eCD, eCB, eBD}
}

func TestReroute_SwitchesWhenTheNewRouteIsCheaper(t *testing.T) {
	// The tail of the current route (e_CD) costs 1. The alternative (e_CB +
	// e_BD) is free, a 100% improvement, so the vehicle should switch.
	s, v, fleet, _, edges := rerouteFixture(t, 0.10)
	fleet.costs[edges[2]] = 0
	fleet.costs[edges[3]] = 0

	// The first reroute check is one RerouteEveryS after departure.
	for i := 0; i < 11; i++ {
		s.Step()
	}

	if v.RouteChanges() != 1 {
		t.Fatalf("RouteChanges() = %d, want 1", v.RouteChanges())
	}
	// The vehicle finishes the edge it is on, then follows the new route.
	want := []int{edges[0], edges[2], edges[3]}
	if len(v.route) != len(want) {
		t.Fatalf("route = %v, want %v", v.route, want)
	}
	for i := range want {
		if v.route[i] != want[i] {
			t.Fatalf("route = %v, want %v", v.route, want)
		}
	}
	if v.state != VehicleRunning {
		t.Errorf("state = %s, want running", v.state)
	}
}

func TestReroute_KeepsTheRouteWhenTheGainIsSmall(t *testing.T) {
	// Every edge costs 1: the alternative (2) is worse than the tail (1), so
	// nothing should change.
	s, v, _, _, _ := rerouteFixture(t, 0.10)

	for i := 0; i < 40; i++ {
		s.Step()
	}

	if v.RouteChanges() != 0 {
		t.Errorf("RouteChanges() = %d, want 0", v.RouteChanges())
	}
}

func TestReroute_HysteresisIgnoresMarginalImprovements(t *testing.T) {
	// Tail e_CD costs 2, the alternative e_CB + e_BD costs 1.9: a 5% gain, below
	// the 10% threshold, so the vehicle should stay put.
	s, v, fleet, _, edges := rerouteFixture(t, 0.10)
	fleet.costs[edges[1]] = 2
	fleet.costs[edges[2]] = 0.95
	fleet.costs[edges[3]] = 0.95

	for i := 0; i < 20; i++ {
		s.Step()
	}

	if v.RouteChanges() != 0 {
		t.Errorf("RouteChanges() = %d, want 0 for a 5%% gain", v.RouteChanges())
	}

	// With the threshold lowered to zero the same comparison does switch.
	s2, v2, fleet2, _, edges2 := rerouteFixture(t, 0)
	fleet2.costs[edges2[1]] = 2
	fleet2.costs[edges2[2]] = 0.95
	fleet2.costs[edges2[3]] = 0.95

	for i := 0; i < 20; i++ {
		s2.Step()
	}

	if v2.RouteChanges() != 1 {
		t.Errorf("RouteChanges() = %d, want 1 with no threshold", v2.RouteChanges())
	}
}

func TestReroute_IsSkippedWhenDisabled(t *testing.T) {
	s, v, _, _, _ := rerouteFixture(t, 0.10)
	s.cfg.RerouteEveryS = 0

	for i := 0; i < 40; i++ {
		s.Step()
	}

	if v.RouteChanges() != 0 {
		t.Errorf("RouteChanges() = %d, want 0 when rerouting is disabled", v.RouteChanges())
	}
}
