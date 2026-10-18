package sim

import (
	"errors"
	"testing"
	"time"

	"fleetflow/internal/engine"
)

type fakeReport struct {
	vehicleID string
	edgeID    string
	speedMPS  float64
}

// fakeFleet records what fleet vehicles ask for and report, so the tests can
// check that fleet and background traffic behave differently. It prices edges
// from its own cost slice, which keeps routing and comparison consistent.
type fakeFleet struct {
	costs      []float64
	routes     map[string][]int
	routeCalls []string
	reports    []fakeReport
	// reportErr, when set, is returned by every Report: the fleet service is
	// unreachable. It still records what was sent, so a test can tell a refused
	// report from one that was never attempted.
	reportErr error
}

func newFakeFleet(edges int) *fakeFleet {
	costs := make([]float64, edges)
	for i := range costs {
		costs[i] = 1
	}
	return &fakeFleet{costs: costs, routes: make(map[string][]int)}
}

// price sums the fake cost of a sequence of edges.
func (f *fakeFleet) price(edges []int) float64 {
	var total float64
	for _, idx := range edges {
		total += f.costs[idx]
	}
	return total
}

func (f *fakeFleet) Route(from, to string, held []int) (engine.RouteAnswer, error) {
	key := from + "->" + to
	f.routeCalls = append(f.routeCalls, key)

	edges, ok := f.routes[key]
	if !ok {
		return engine.RouteAnswer{}, errors.New("fake has no route for " + key)
	}
	answer := engine.RouteAnswer{
		Path:    &engine.Path{Edges: edges, CostS: f.price(edges)},
		Version: 1,
	}
	if held != nil {
		answer.HeldCostS = f.price(held)
		answer.HeldPriced = true
	}
	return answer, nil
}

func (f *fakeFleet) Report(vehicleID, edgeID string, speedMPS float64, _ time.Time) error {
	f.reports = append(f.reports, fakeReport{vehicleID: vehicleID, edgeID: edgeID, speedMPS: speedMPS})
	return f.reportErr
}

// TestReportSpeeds_CountsWhatTheServiceRefused: one dropped report only makes
// the weights slightly stale, so a run does not stop for it -- but a run whose
// arm can never report has no dynamic weights at all, and that is what an
// out-of-process fleet service looks like when the server is down or the key is
// wrong. Without this count the comparison would read "nothing to report"
// rather than "nothing reported", and the two arms would look identical for a
// reason that has nothing to do with routing.
func TestReportSpeeds_CountsWhatTheServiceRefused(t *testing.T) {
	s, g := newFixtureSim(t, 1, 1)
	v := soloTrip(t, s, "A", "D", true)
	v.departS = 0

	fleet := newFakeFleet(len(g.Edges))
	fleet.routes["A->D"] = []int{edgeIndexByID(t, g, "e_AB"), edgeIndexByID(t, g, "e_BD")}
	fleet.reportErr = errors.New("fleet service is unreachable")
	s.SetFleetService(fleet)

	s.Step()
	s.Step()

	if len(fleet.reports) != 2 {
		t.Fatalf("got %d reports, want 2: the run must keep trying", len(fleet.reports))
	}
	if got := s.Collect().ReportsFailed; got != 2 {
		t.Errorf("ReportsFailed = %d, want 2", got)
	}

	// A service that accepts them again clears nothing by itself, but it must
	// not start counting successful reports as failures.
	fleet.reportErr = nil
	s.Step()

	if got := s.Collect().ReportsFailed; got != 2 {
		t.Errorf("ReportsFailed = %d after a report the service took, want 2", got)
	}
}

func TestFleetVehicleAsksTheServiceForARoute(t *testing.T) {
	s, g := newFixtureSim(t, 1, 1) // every vehicle is a fleet vehicle
	v := soloTrip(t, s, "A", "D", true)
	v.departS = 0

	fleet := newFakeFleet(len(g.Edges))
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

	fleet := newFakeFleet(len(g.Edges))
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
		if r.vehicleID != v.ID() {
			t.Errorf("reported as vehicle %q, want %q", r.vehicleID, v.ID())
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
	s, g := newFixtureSim(t, 1, 0) // no fleet vehicles
	v := soloTrip(t, s, "A", "D", false)
	v.departS = 0

	fleet := newFakeFleet(len(g.Edges))
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
