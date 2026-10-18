package engine

import (
	"testing"
	"time"
)

// TestReset_ReturnsTheWeightsToFreeFlow is the capability a simulation run needs:
// whatever congestion the engine was holding is gone, and the next query routes on
// an empty network.
func TestReset_ReturnsTheWeightsToFreeFlow(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	_, index, ok := g.EdgeByID("e_AB")
	if !ok {
		t.Fatal("the fixture graph has no e_AB edge")
	}
	freeFlow := e.ActiveMetric().Cost(index)

	// Congest the edge the way a previous run would have, and confirm it moved.
	if err := e.Report("e_AB", 1, time.Now()); err != nil {
		t.Fatalf("report: %v", err)
	}
	e.Customize()
	if congested := e.ActiveMetric().Cost(index); congested <= freeFlow {
		t.Fatalf("cost after a crawl = %v, want more than free flow %v", congested, freeFlow)
	}

	e.Reset()

	if got := e.ActiveMetric().Cost(index); got != freeFlow {
		t.Errorf("cost after a reset = %v, want free flow %v", got, freeFlow)
	}
}

// TestReset_PublishesANewSnapshot keeps the reset visible to a query that was
// already holding the previous snapshot: the version moves, so a client can tell
// its route predates the reset.
func TestReset_PublishesANewSnapshot(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	before := e.ActiveMetric().Version()
	e.Reset()

	after := e.ActiveMetric()
	if after.Version() <= before {
		t.Errorf("version after a reset = %d, want more than %d", after.Version(), before)
	}
	if got, want := after.Len(), len(g.Edges); got != want {
		t.Errorf("metric length = %d, want %d", got, want)
	}
}

// TestReset_LeavesABaselineAlone keeps the comparison arm honest: its metric is
// fixed to free-flow times by definition, so there is nothing to clear and no new
// snapshot to publish.
func TestReset_LeavesABaselineAlone(t *testing.T) {
	g := loadFixture(t)
	e, err := NewBaseline(g, NewDijkstraRouter(g))
	if err != nil {
		t.Fatalf("failed to build the baseline engine: %v", err)
	}

	before := e.ActiveMetric().Version()
	e.Reset()

	if got := e.ActiveMetric().Version(); got != before {
		t.Errorf("baseline version = %d after a reset, want it unchanged at %d", got, before)
	}
}
