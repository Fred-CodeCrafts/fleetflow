package graph

import (
	"errors"
	"strings"
	"testing"
)

const validGraphJSON = `{
  "nodes": [
    {"id": "A", "lat": -6.2088, "lon": 106.8456},
    {"id": "B", "lat": -6.2188, "lon": 106.8556}
  ],
  "edges": [
    {"id": "e1", "from": "A", "to": "B", "length_m": 500.0, "speed_limit_mps": 13.89, "lanes": 2}
  ]
}`

const missingNodeGraphJSON = `{
  "nodes": [
    {"id": "A", "lat": -6.2088, "lon": 106.8456}
  ],
  "edges": [
    {"id": "e1", "from": "A", "to": "B", "length_m": 500.0, "speed_limit_mps": 13.89, "lanes": 2}
  ]
}`

const invalidLengthGraphJSON = `{
  "nodes": [
    {"id": "A", "lat": -6.2088, "lon": 106.8456},
    {"id": "B", "lat": -6.2188, "lon": 106.8556}
  ],
  "edges": [
    {"id": "e1", "from": "A", "to": "B", "length_m": -100.0, "speed_limit_mps": 13.89, "lanes": 2}
  ]
}`

func TestLoadFromJSON_Valid(t *testing.T) {
	g, err := LoadFromJSON(strings.NewReader(validGraphJSON))
	if err != nil {
		t.Fatalf("unexpected error loading valid graph: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(g.Edges))
	}
}

func TestLoadFromJSON_MissingNode(t *testing.T) {
	_, err := LoadFromJSON(strings.NewReader(missingNodeGraphJSON))
	if err == nil {
		t.Fatal("expected error for missing node, got nil")
	}
	if !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("expected ErrNodeNotFound, got: %v", err)
	}
}

func TestLoadFromJSON_InvalidLength(t *testing.T) {
	_, err := LoadFromJSON(strings.NewReader(invalidLengthGraphJSON))
	if err == nil {
		t.Fatal("expected error for negative edge length, got nil")
	}
	if !errors.Is(err, ErrInvalidEdgeLength) {
		t.Errorf("expected ErrInvalidEdgeLength, got: %v", err)
	}
}

func TestLoadFromFile_Fixture(t *testing.T) {
	g, err := LoadFromFile("../../data/test_graph.json")
	if err != nil {
		t.Fatalf("failed to load test graph fixture: %v", err)
	}
	if len(g.Nodes) != 4 {
		t.Errorf("expected 4 nodes in fixture, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 7 {
		t.Errorf("expected 7 edges in fixture, got %d", len(g.Edges))
	}

	eAB, _, found := g.EdgeByID("e_AB")
	if !found {
		t.Fatal("expected e_AB to be found in fixture")
	}
	if eAB.FreeFlowS <= 0 || eAB.Capacity <= 0 {
		t.Errorf("expected derived values on e_AB, got FreeFlowS=%f, Capacity=%f", eAB.FreeFlowS, eAB.Capacity)
	}
}
