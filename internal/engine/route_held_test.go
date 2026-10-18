package engine

import (
	"errors"
	"testing"
	"time"
)

// TestRouteWithHeld_PricesTheHeldPathOnTheRouteSnapshot is the point of
// RouteWithHeld: the path and the held cost are read from one snapshot, so a
// caller can compare them. Congesting a held edge between two queries must move
// the held cost and the version together, and both must match the snapshot the
// path was computed from.
func TestRouteWithHeld_PricesTheHeldPathOnTheRouteSnapshot(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	held := []int{edgeIndex(t, g, "e_AB"), edgeIndex(t, g, "e_BD")}

	before, err := e.RouteWithHeld("A", "D", held)
	if err != nil {
		t.Fatalf("RouteWithHeld() = %v", err)
	}
	if !before.HeldPriced {
		t.Fatal("HeldPriced = false for a held path that was sent")
	}
	if want := e.ActiveMetric().Version(); before.Version != want {
		t.Errorf("version = %d, want the active snapshot %d", before.Version, want)
	}
	wantBefore := g.Edges[held[0]].FreeFlowS + g.Edges[held[1]].FreeFlowS
	if !almostEqual(before.HeldCostS, wantBefore) {
		t.Errorf("held cost = %f, want the free-flow sum %f", before.HeldCostS, wantBefore)
	}

	// Congest a held edge and publish, so the held cost has to move.
	for i := 0; i < 5; i++ {
		if err := e.Report("e_BD", 0, time.Now()); err != nil {
			t.Fatalf("unexpected report error: %v", err)
		}
	}
	e.Customize()

	after, err := e.RouteWithHeld("A", "D", held)
	if err != nil {
		t.Fatalf("RouteWithHeld() = %v", err)
	}

	m := e.ActiveMetric()
	if after.Version != m.Version() {
		t.Fatalf("version = %d, want the active snapshot %d", after.Version, m.Version())
	}
	if !almostEqual(after.HeldCostS, m.Cost(held[0])+m.Cost(held[1])) {
		t.Errorf("held cost = %f, want %f from the answering snapshot",
			after.HeldCostS, m.Cost(held[0])+m.Cost(held[1]))
	}
	if after.HeldCostS <= before.HeldCostS {
		t.Errorf("held cost = %f after congestion, want more than %f", after.HeldCostS, before.HeldCostS)
	}

	// The path must come from that same snapshot, not a fresher one: its cost
	// is the sum of the answering metric over its own edges.
	var pathCost float64
	for _, idx := range after.Path.Edges {
		pathCost += m.Cost(idx)
	}
	if !almostEqual(after.Path.CostS, pathCost) {
		t.Errorf("path cost = %f, want %f from the answering snapshot", after.Path.CostS, pathCost)
	}
}

func TestRouteWithHeld_WithoutAHeldPathDoesNotPriceOne(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	answer, err := e.RouteWithHeld("A", "D", nil)
	if err != nil {
		t.Fatalf("RouteWithHeld() = %v", err)
	}
	if answer.HeldPriced {
		t.Error("HeldPriced = true for a query that sent no held path")
	}
	if answer.HeldCostS != 0 {
		t.Errorf("held cost = %f, want 0 when nothing was asked for", answer.HeldCostS)
	}
}

// An empty, non-nil held path is a real answer — the caller holds nothing, so
// the cost is zero — and is told apart from not asking at all.
func TestRouteWithHeld_EmptyHeldPathCostsZero(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	answer, err := e.RouteWithHeld("A", "D", []int{})
	if err != nil {
		t.Fatalf("RouteWithHeld() = %v", err)
	}
	if !answer.HeldPriced {
		t.Error("HeldPriced = false for an empty held path")
	}
	if answer.HeldCostS != 0 {
		t.Errorf("held cost = %f, want 0 for an empty held path", answer.HeldCostS)
	}
}

func TestRouteWithHeld_RejectsBadIndexes(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	for _, idx := range []int{-1, len(g.Edges), 1 << 20} {
		if _, err := e.RouteWithHeld("A", "D", []int{idx}); !errors.Is(err, ErrEdgeNotFound) {
			t.Errorf("held index %d: err = %v, want ErrEdgeNotFound", idx, err)
		}
	}
}
