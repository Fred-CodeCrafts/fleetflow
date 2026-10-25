package engine

import (
	"errors"
	"math"
	"testing"

	"fleetflow/internal/graph"
)

const (
	fixtureSpeed   = 13.89
	fixtureSlowMPS = 11.11
)

// loadFixture builds the four-node test network in code. It used to be read from
// data/test_graph.json; the network is seven edges, so building it here removes a
// data file that only the tests ever loaded and keeps a clone reproducible.
func loadFixture(t *testing.T) *graph.Graph {
	t.Helper()
	g := graph.New()
	nodes := []graph.Node{
		{ID: "A", Lat: -6.1754, Lon: 106.8272},
		{ID: "B", Lat: -6.1767, Lon: 106.8306},
		{ID: "C", Lat: -6.1874, Lon: 106.8236},
		{ID: "D", Lat: -6.1950, Lon: 106.8230},
	}
	edges := []graph.Edge{
		{ID: "e_AB", From: "A", To: "B", LengthM: 400, SpeedLimitMPS: fixtureSpeed, Lanes: 2},
		{ID: "e_BD", From: "B", To: "D", LengthM: 600, SpeedLimitMPS: fixtureSpeed, Lanes: 2},
		{ID: "e_AC", From: "A", To: "C", LengthM: 500, SpeedLimitMPS: fixtureSpeed, Lanes: 2},
		{ID: "e_CD", From: "C", To: "D", LengthM: 400, SpeedLimitMPS: fixtureSpeed, Lanes: 2},
		{ID: "e_BC", From: "B", To: "C", LengthM: 300, SpeedLimitMPS: fixtureSlowMPS, Lanes: 1},
		{ID: "e_CB", From: "C", To: "B", LengthM: 300, SpeedLimitMPS: fixtureSlowMPS, Lanes: 1},
		{ID: "e_DA", From: "D", To: "A", LengthM: 900, SpeedLimitMPS: fixtureSpeed, Lanes: 3},
	}
	for _, n := range nodes {
		if err := g.AddNode(n); err != nil {
			t.Fatalf("add node %s: %v", n.ID, err)
		}
	}
	for _, e := range edges {
		if err := g.AddEdge(e); err != nil {
			t.Fatalf("add edge %s: %v", e.ID, err)
		}
	}
	return g
}

// freeFlowMetric builds a metric that costs every edge at its free-flow time.
func freeFlowMetric(g *graph.Graph, version uint64) *Metric {
	costs := make([]float64, len(g.Edges))
	for i, e := range g.Edges {
		costs[i] = e.FreeFlowS
	}
	return NewMetric(costs, version)
}

func edgeIDs(g *graph.Graph, indices []int) []string {
	ids := make([]string, len(indices))
	for i, idx := range indices {
		ids[i] = g.Edges[idx].ID
	}
	return ids
}

func TestDijkstra_FreeFlowShortestPaths(t *testing.T) {
	g := loadFixture(t)
	m := freeFlowMetric(g, 1)

	tests := []struct {
		name  string
		from  string
		to    string
		edges []string
	}{
		{name: "direct edge wins", from: "A", to: "B", edges: []string{"e_AB"}},
		{name: "faster pair of edges beats the direct route", from: "A", to: "D", edges: []string{"e_AC", "e_CD"}},
		{name: "reverse direction uses the D to A edge", from: "B", to: "A", edges: []string{"e_BD", "e_DA"}},
		{name: "origin equals destination", from: "A", to: "A", edges: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := NewDijkstraRouter(g).Route(m, tt.from, tt.to)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := edgeIDs(g, path.Edges)
			if len(got) != len(tt.edges) {
				t.Fatalf("edges = %v, want %v", got, tt.edges)
			}
			for i := range got {
				if got[i] != tt.edges[i] {
					t.Fatalf("edges = %v, want %v", got, tt.edges)
				}
			}

			// The reported cost must equal the sum of the chosen edges.
			var sum float64
			for _, idx := range path.Edges {
				sum += g.Edges[idx].FreeFlowS
			}
			if math.Abs(path.CostS-sum) > 1e-9 {
				t.Errorf("CostS = %f, want %f", path.CostS, sum)
			}
		})
	}
}

func TestDijkstra_RespectsTheMetric(t *testing.T) {
	g := loadFixture(t)

	// Pollute the A->C edge: the route should now prefer A->B->D.
	costs := make([]float64, len(g.Edges))
	for i, e := range g.Edges {
		costs[i] = e.FreeFlowS
		if e.ID == "e_AC" {
			costs[i] = 1000
		}
	}
	m := NewMetric(costs, 2)

	path, err := NewDijkstraRouter(g).Route(m, "A", "D")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := edgeIDs(g, path.Edges)
	want := []string{"e_AB", "e_BD"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("edges = %v, want %v", got, want)
	}
}

func TestDijkstra_UnknownNode(t *testing.T) {
	g := loadFixture(t)
	m := freeFlowMetric(g, 1)
	router := NewDijkstraRouter(g)

	if _, err := router.Route(m, "Z", "A"); !errors.Is(err, ErrUnknownNode) {
		t.Errorf("expected ErrUnknownNode for a bad origin, got %v", err)
	}
	if _, err := router.Route(m, "A", "Z"); !errors.Is(err, ErrUnknownNode) {
		t.Errorf("expected ErrUnknownNode for a bad destination, got %v", err)
	}
}

func TestDijkstra_NoPath(t *testing.T) {
	g := graph.New()
	mustAddNode(t, g, "A")
	mustAddNode(t, g, "B")
	if err := g.AddEdge(graph.Edge{ID: "e_AB", From: "A", To: "B", LengthM: 100, SpeedLimitMPS: 10, Lanes: 1}); err != nil {
		t.Fatalf("unexpected error adding edge: %v", err)
	}

	_, err := NewDijkstraRouter(g).Route(freeFlowMetric(g, 1), "B", "A")
	if !errors.Is(err, ErrNoPath) {
		t.Errorf("expected ErrNoPath, got %v", err)
	}
}

func mustAddNode(t *testing.T, g *graph.Graph, id string) {
	t.Helper()
	if err := g.AddNode(graph.Node{ID: id}); err != nil {
		t.Fatalf("unexpected error adding node %s: %v", id, err)
	}
}
