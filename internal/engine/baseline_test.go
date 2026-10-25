package engine

import (
	"errors"
	"testing"
	"time"

	"fleetflow/internal/graph"
)

func newTestBaseline(t *testing.T, g *graph.Graph) *Engine {
	t.Helper()
	e, err := NewBaseline(g, NewDijkstraRouter(g))
	if err != nil {
		t.Fatalf("unexpected error creating baseline engine: %v", err)
	}
	return e
}

func TestNewBaseline_FixesTheMetricAtFreeFlow(t *testing.T) {
	g := loadFixture(t)
	e := newTestBaseline(t, g)

	ac := edgeIndex(t, g, "e_AC")
	first := e.ActiveMetric()
	if got, want := first.Cost(ac), g.Edges[ac].FreeFlowS; !almostEqual(got, want) {
		t.Errorf("baseline cost = %f, want free-flow %f", got, want)
	}

	// Reports and customization must not move the baseline.
	for i := 0; i < 5; i++ {
		if err := e.Report("e_AC", 0, time.Now()); err != nil {
			t.Fatalf("unexpected report error: %v", err)
		}
	}
	e.DecayStale(time.Now())
	e.Customize()

	if got := e.ActiveMetric().Version(); got != first.Version() {
		t.Errorf("baseline version moved from %d to %d", first.Version(), got)
	}
	if got, want := e.ActiveMetric().Cost(ac), g.Edges[ac].FreeFlowS; !almostEqual(got, want) {
		t.Errorf("baseline cost = %f, want free-flow %f after reports", got, want)
	}
}

func TestBaseline_KeepsTheStaticRoute(t *testing.T) {
	g := loadFixture(t)
	e := newTestBaseline(t, g)

	if err := e.Report("e_AC", 0, time.Now()); err != nil {
		t.Fatalf("unexpected report error: %v", err)
	}
	e.Customize()

	p, err := e.Route("A", "D")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := edgeIDs(g, p.Edges)
	if len(got) != 2 || got[0] != "e_AC" || got[1] != "e_CD" {
		t.Errorf("baseline route = %v, want the static [e_AC e_CD]", got)
	}
}

func TestBaseline_IssuesAnErrorForUnknownEdges(t *testing.T) {
	g := loadFixture(t)
	e := newTestBaseline(t, g)

	if err := e.Report("nope", 5, time.Now()); !errors.Is(err, ErrEdgeNotFound) {
		t.Errorf("expected ErrEdgeNotFound, got %v", err)
	}
}

func TestBaseline_RouteStillReportsUnknownNodes(t *testing.T) {
	g := loadFixture(t)
	e := newTestBaseline(t, g)

	if _, err := e.Route("A", "Z"); !errors.Is(err, ErrUnknownNode) {
		t.Errorf("expected ErrUnknownNode, got %v", err)
	}
}
