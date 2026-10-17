package graph

import (
	"errors"
	"testing"
)

func TestGraph_AddNodeAndEdge(t *testing.T) {
	g := New()

	n1 := Node{ID: "n1", Lat: -6.2, Lon: 106.8}
	n2 := Node{ID: "n2", Lat: -6.21, Lon: 106.81}

	if err := g.AddNode(n1); err != nil {
		t.Fatalf("unexpected error adding n1: %v", err)
	}
	if err := g.AddNode(n2); err != nil {
		t.Fatalf("unexpected error adding n2: %v", err)
	}

	// Duplicate node error
	if err := g.AddNode(n1); !errors.Is(err, ErrDuplicateNode) {
		t.Fatalf("expected ErrDuplicateNode, got %v", err)
	}

	// Empty node ID error
	if err := g.AddNode(Node{}); err == nil {
		t.Fatal("expected error for empty node ID, got nil")
	}

	// Add edge with non-existent node
	badEdge := Edge{ID: "e_bad", From: "n1", To: "missing", LengthM: 500, SpeedLimitMPS: 15, Lanes: 2}
	if err := g.AddEdge(badEdge); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}

	// Add valid edge
	e1 := Edge{ID: "e1", From: "n1", To: "n2", LengthM: 500, SpeedLimitMPS: 15, Lanes: 2}
	if err := g.AddEdge(e1); err != nil {
		t.Fatalf("unexpected error adding edge: %v", err)
	}

	// Duplicate edge error
	if err := g.AddEdge(e1); !errors.Is(err, ErrDuplicateEdge) {
		t.Fatalf("expected ErrDuplicateEdge, got %v", err)
	}

	// Check edge retrieval
	edge, idx, found := g.EdgeByID("e1")
	if !found {
		t.Fatal("expected edge e1 to be found")
	}
	if idx != 0 || edge.ID != "e1" {
		t.Fatalf("unexpected edge data: idx=%d, edge=%+v", idx, edge)
	}

	// Outgoing edges
	out := g.OutgoingEdges("n1")
	if len(out) != 1 || out[0].ID != "e1" {
		t.Fatalf("unexpected outgoing edges for n1: %+v", out)
	}

	outN2 := g.OutgoingEdges("n2")
	if len(outN2) != 0 {
		t.Fatalf("expected 0 outgoing edges for n2, got %d", len(outN2))
	}
}
