package engine

import (
	"testing"
	"time"

	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
)

// statesFor returns one fresh EdgeState per edge of g.
func statesFor(g *graph.Graph) []*weights.EdgeState {
	states := make([]*weights.EdgeState, len(g.Edges))
	for i := range states {
		states[i] = weights.NewEdgeState()
	}
	return states
}

func TestBuildDynamicCosts_QuietEdgesCostFreeFlowTime(t *testing.T) {
	g := loadFixture(t)
	p := weights.DefaultParams()

	costs := buildDynamicCosts(g, statesFor(g), p)

	for i, e := range g.Edges {
		if costs[i] != e.FreeFlowS {
			t.Errorf("quiet edge %s: cost = %f, want free-flow %f", e.ID, costs[i], e.FreeFlowS)
		}
	}
}

func TestBuildDynamicCosts_CongestedEdgeIsPenalised(t *testing.T) {
	g := loadFixture(t)
	p := weights.DefaultParams()
	states := statesFor(g)

	// Report a stopped vehicle on e_AB; gamma 1 takes the raw congestion.
	target := edgeIndex(t, g, "e_AB")
	states[target].Observe(0, g.Edges[target].SpeedLimitMPS, 1, time.Now())

	costs := buildDynamicCosts(g, states, p)

	want := g.Edges[target].FreeFlowS * (1 + p.Alpha) // rho = 1, so the penalty is alpha
	if !almostEqual(costs[target], want) {
		t.Errorf("congested edge cost = %f, want %f", costs[target], want)
	}

	// Every other edge must still be at free-flow.
	for i, e := range g.Edges {
		if i == target {
			continue
		}
		if costs[i] != e.FreeFlowS {
			t.Errorf("edge %s: cost = %f, want free-flow %f", e.ID, costs[i], e.FreeFlowS)
		}
	}
}

func TestBuildDynamicCosts_ReturnsAFreshSlice(t *testing.T) {
	g := loadFixture(t)
	p := weights.DefaultParams()
	states := statesFor(g)

	first := buildDynamicCosts(g, states, p)

	// Congest everything, then rebuild: the first slice must not change.
	for i := range states {
		states[i].Observe(0, 10, 1, time.Now())
	}
	second := buildDynamicCosts(g, states, p)

	for i := range first {
		if first[i] != g.Edges[i].FreeFlowS {
			t.Errorf("previous snapshot changed at edge %d: %f", i, first[i])
		}
		if second[i] <= first[i] {
			t.Errorf("edge %d: congested cost %f should exceed free-flow %f", i, second[i], first[i])
		}
	}
}

// edgeIndex returns the index of the edge with the given ID.
func edgeIndex(t *testing.T, g *graph.Graph, id string) int {
	t.Helper()
	_, idx, ok := g.EdgeByID(id)
	if !ok {
		t.Fatalf("edge %s not found in graph", id)
	}
	return idx
}

func almostEqual(a, b float64) bool {
	const eps = 1e-9
	diff := a - b
	return diff < eps && diff > -eps
}
