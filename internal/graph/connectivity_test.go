package graph

import "testing"

// buildGraph assembles a graph from edge tuples: from, to, and an ID.
func buildGraph(t *testing.T, ids []string, edges [][3]string) *Graph {
	t.Helper()
	g := New()
	for _, id := range ids {
		if err := g.AddNode(Node{ID: id}); err != nil {
			t.Fatalf("AddNode(%q): %v", id, err)
		}
	}
	for _, e := range edges {
		if err := g.AddEdge(Edge{ID: e[2], From: e[0], To: e[1], LengthM: 100, SpeedLimitMPS: 10, Lanes: 1}); err != nil {
			t.Fatalf("AddEdge(%q): %v", e[2], err)
		}
	}
	return g
}

func TestKeepLargestStronglyConnectedKeepsMainComponent(t *testing.T) {
	// A three-node ring plus an island reachable only one way: X -> Y with no
	// way back. Y cannot be left, so a trip ending there could never complete,
	// and the pair is not strongly connected with the ring.
	g := buildGraph(t,
		[]string{"A", "B", "C", "X", "Y"},
		[][3]string{
			{"A", "B", "ab"}, {"B", "C", "bc"}, {"C", "A", "ca"},
			{"A", "X", "ax"}, {"X", "Y", "xy"},
		},
	)

	removed := g.KeepLargestStronglyConnected()
	if removed != 2 {
		t.Fatalf("removed %d nodes, want 2", removed)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("kept %d nodes, want 3", len(g.Nodes))
	}
	for _, id := range []string{"A", "B", "C"} {
		if _, ok := g.Nodes[id]; !ok {
			t.Errorf("node %q was pruned, want kept", id)
		}
	}
	for _, id := range []string{"X", "Y"} {
		if _, ok := g.Nodes[id]; ok {
			t.Errorf("node %q was kept, want pruned", id)
		}
	}
	if len(g.Edges) != 3 {
		t.Fatalf("kept %d edges, want 3", len(g.Edges))
	}
	// The pruned graph must be usable: lookups still resolve and the ring still
	// routes.
	if _, _, ok := g.EdgeByID("ab"); !ok {
		t.Error("edge ab missing after pruning")
	}
	if _, _, ok := g.EdgeByID("ax"); ok {
		t.Error("edge ax survived pruning, want removed")
	}
	if got := len(g.OutgoingEdgeIndices("A")); got != 1 {
		t.Errorf("node A has %d outgoing edges, want 1", got)
	}
}

func TestKeepLargestStronglyConnectedLeavesConnectedGraphAlone(t *testing.T) {
	g := buildGraph(t,
		[]string{"A", "B"},
		[][3]string{{"A", "B", "ab"}, {"B", "A", "ba"}},
	)
	if removed := g.KeepLargestStronglyConnected(); removed != 0 {
		t.Fatalf("removed %d nodes from a connected graph, want 0", removed)
	}
	if len(g.Nodes) != 2 || len(g.Edges) != 2 {
		t.Fatalf("graph changed: %d nodes, %d edges", len(g.Nodes), len(g.Edges))
	}
}

func TestKeepLargestStronglyConnectedPrefersLargestTie(t *testing.T) {
	// Two rings of different size; the bigger one must win regardless of the
	// order nodes happen to be iterated in.
	g := buildGraph(t,
		[]string{"A", "B", "C", "D", "E"},
		[][3]string{
			{"A", "B", "ab"}, {"B", "A", "ba"},
			{"C", "D", "cd"}, {"D", "E", "de"}, {"E", "C", "ec"},
		},
	)
	g.KeepLargestStronglyConnected()
	if _, ok := g.Nodes["C"]; !ok {
		t.Fatal("the three-node ring was pruned in favour of the two-node ring")
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("kept %d nodes, want 3", len(g.Nodes))
	}
}

func TestKeepLargestStronglyConnectedOnEmptyGraph(t *testing.T) {
	g := New()
	if removed := g.KeepLargestStronglyConnected(); removed != 0 {
		t.Fatalf("removed %d nodes from an empty graph, want 0", removed)
	}
}
