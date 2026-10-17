package sim

import (
	"errors"
	"testing"
	"time"

	"fleetflow/internal/engine"
)

type fakeReport struct {
	edgeID   string
	speedMPS float64
}

// fakeFleet records what fleet vehicles ask for and report, so the tests can
// check that fleet and background traffic behave differently.
type fakeFleet struct {
	routes     map[string][]int
	routeCalls []string
	reports    []fakeReport
}

func newFakeFleet() *fakeFleet {
	return &fakeFleet{routes: make(map[string][]int)}
}

func (f *fakeFleet) Route(from, to string) (*engine.Path, error) {
	key := from + "->" + to
	f.routeCalls = append(f.routeCalls, key)

	edges, ok := f.routes[key]
	if !ok {
		return nil, errors.New("fake has no route for " + key)
	}
	return &engine.Path{Edges: edges}, nil
}

func (f *fakeFleet) Report(edgeID string, speedMPS float64, _ time.Time) error {
	f.reports = append(f.reports, fakeReport{edgeID: edgeID, speedMPS: speedMPS})
	return nil
}

func TestFleetVehicleAsksTheServiceForARoute(t *testing.T) {
	s, g := newFixtureSim(t, 1, 1) // every vehicle is a fleet vehicle
	v := soloTrip(t, s, "A", "D", true)
	v.departS = 0

	fleet := newFakeFleet()
	want := []int{edgeIndexByID(t, g, "e_AB"), edgeIndexByID(t, g, "e_BD")}
	fleet.routes["A->D"] = want
	s.SetFleetService(fleet)

	s.Step()

	if len(fleet.routeCalls) != 1 || fleet.routeCalls[0] != "A->D" {
		t.Fatalf("route calls = %v, want [A->D]", fleet.routeCalls)
	}
	if len(v.route) != 2 || v.route[0] != want[0] || v.route[1] != want[1] {
		t.Errorf("route = %v, want %v", v.route, want)
	}
}

func TestFleetVehicleReportsTheSpeedItExperiences(t *testing.T) {
	s, g := newFixtureSim(t, 1, 1)
	v := soloTrip(t, s, "A", "D", true)
	v.departS = 0

	fleet := newFakeFleet()
	fleet.routes["A->D"] = []int{edgeIndexByID(t, g, "e_AB"), edgeIndexByID(t, g, "e_BD")}
	s.SetFleetService(fleet)

	s.Step() // departs and drives on e_AB
	s.Step() // drives further on e_AB

	if len(fleet.reports) != 2 {
		t.Fatalf("got %d reports, want 2", len(fleet.reports))
	}
	for _, r := range fleet.reports {
		if r.edgeID != "e_AB" {
			t.Errorf("reported on %s, want e_AB", r.edgeID)
		}
		if r.speedMPS <= 0 {
			t.Errorf("reported speed %f, want a positive speed", r.speedMPS)
		}
	}

	// Once the vehicle arrives it stops reporting.
	for i := 0; i < 500 && v.State() == VehicleRunning; i++ {
		s.Step()
	}
	if v.State() != VehicleArrived {
		t.Fatalf("State() = %s, want arrived", v.State())
	}
	after := len(fleet.reports)

	s.Step()

	if len(fleet.reports) != after {
		t.Errorf("got %d reports after arrival, want no new ones (%d)", len(fleet.reports), after)
	}
}

func TestBackgroundVehiclesDoNotContactTheService(t *testing.T) {
	s, _ := newFixtureSim(t, 1, 0) // no fleet vehicles
	v := soloTrip(t, s, "A", "D", false)
	v.departS = 0

	fleet := newFakeFleet()
	s.SetFleetService(fleet)

	for i := 0; i < 100 && v.State() != VehicleArrived; i++ {
		s.Step()
	}

	if len(fleet.routeCalls) != 0 {
		t.Errorf("route calls = %v, want none for background traffic", fleet.routeCalls)
	}
	if len(fleet.reports) != 0 {
		t.Errorf("reports = %v, want none for background traffic", fleet.reports)
	}
}

func TestFleetVehicleWithoutAServiceFallsBackToStaticRoutes(t *testing.T) {
	s, g := newFixtureSim(t, 1, 1) // a fleet vehicle with no service attached
	v := soloTrip(t, s, "A", "D", true)
	v.departS = 0

	s.Step()

	want := []int{edgeIndexByID(t, g, "e_AC"), edgeIndexByID(t, g, "e_CD")}
	if len(v.route) != 2 || v.route[0] != want[0] || v.route[1] != want[1] {
		t.Errorf("route = %v, want the static %v", v.route, want)
	}
}
