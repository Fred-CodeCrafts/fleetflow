package engine

import (
	"errors"
	"testing"
	"time"

	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
)

func newTestEngine(t *testing.T, g *graph.Graph) *Engine {
	t.Helper()
	e, err := New(g, NewDijkstraRouter(g), weights.DefaultParams())
	if err != nil {
		t.Fatalf("unexpected error creating engine: %v", err)
	}
	return e
}

func TestNew_PublishesFreeFlowMetric(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	m := e.ActiveMetric()
	if m == nil {
		t.Fatal("expected an initial metric, got nil")
	}
	if m.Version() != 1 {
		t.Errorf("initial version = %d, want 1", m.Version())
	}
	if got, want := m.Len(), len(g.Edges); got != want {
		t.Errorf("metric length = %d, want %d", got, want)
	}

	p, err := e.Route("A", "D")
	if err != nil {
		t.Fatalf("unexpected error routing before any report: %v", err)
	}
	if got := edgeIDs(g, p.Edges); len(got) != 2 || got[0] != "e_AC" || got[1] != "e_CD" {
		t.Errorf("initial route = %v, want [e_AC e_CD]", got)
	}
}

func TestNew_RejectsBadInput(t *testing.T) {
	g := loadFixture(t)

	if _, err := New(g, nil, weights.DefaultParams()); err == nil {
		t.Error("expected an error for a nil router")
	}
	if _, err := New(nil, NewDijkstraRouter(g), weights.DefaultParams()); err == nil {
		t.Error("expected an error for a nil graph")
	}

	bad := weights.DefaultParams()
	bad.Beta = -1
	if _, err := New(g, NewDijkstraRouter(g), bad); !errors.Is(err, weights.ErrInvalidBeta) {
		t.Errorf("expected ErrInvalidBeta, got %v", err)
	}
}

func TestReport_UnknownEdge(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	if err := e.Report("nope", 5, time.Now()); !errors.Is(err, ErrEdgeNotFound) {
		t.Errorf("expected ErrEdgeNotFound, got %v", err)
	}
}

func TestCustomize_PublishesANewerVersion(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)
	first := e.ActiveMetric()

	second := e.Customize()

	if second.Version() != first.Version()+1 {
		t.Errorf("version went from %d to %d, want +1", first.Version(), second.Version())
	}
	if e.ActiveMetric() != second {
		t.Error("expected the new snapshot to be active")
	}
}

func TestRoute_UsesTheLatestSnapshot(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	// Congest the A->C edge hard, then publish.
	for i := 0; i < 5; i++ {
		if err := e.Report("e_AC", 0, time.Now()); err != nil {
			t.Fatalf("unexpected report error: %v", err)
		}
	}
	e.Customize()

	p, err := e.Route("A", "D")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := edgeIDs(g, p.Edges)
	if len(got) != 2 || got[0] != "e_AB" || got[1] != "e_BD" {
		t.Errorf("route = %v, want [e_AB e_BD] after e_AC was congested", got)
	}
}

func TestCustomize_LeavesOldSnapshotsIntact(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)
	old := e.ActiveMetric()

	for i := 0; i < 5; i++ {
		if err := e.Report("e_AC", 0, time.Now()); err != nil {
			t.Fatalf("unexpected report error: %v", err)
		}
	}
	e.Customize()

	// A query holding the old snapshot keeps seeing the old costs.
	ac := edgeIndex(t, g, "e_AC")
	if got, want := old.Cost(ac), g.Edges[ac].FreeFlowS; !almostEqual(got, want) {
		t.Errorf("old snapshot cost = %f, want %f", got, want)
	}
	if got := e.ActiveMetric().Cost(ac); got <= old.Cost(ac) {
		t.Errorf("new snapshot cost %f should exceed the old %f", got, old.Cost(ac))
	}
}
