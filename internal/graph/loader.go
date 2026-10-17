package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	ErrInvalidEdgeLength = errors.New("edge length must be positive")
	ErrInvalidSpeedLimit = errors.New("edge speed limit must be positive")
	ErrInvalidLanes      = errors.New("edge lanes must be positive")
	ErrEmptyGraph        = errors.New("graph must contain at least one node and edge")
)

type graphJSON struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// LoadFromJSON parses and validates graph data from an io.Reader.
func LoadFromJSON(r io.Reader) (*Graph, error) {
	var raw graphJSON
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode graph JSON: %w", err)
	}

	if len(raw.Nodes) == 0 || len(raw.Edges) == 0 {
		return nil, ErrEmptyGraph
	}

	g := New()
	for _, n := range raw.Nodes {
		if err := g.AddNode(n); err != nil {
			return nil, fmt.Errorf("invalid node: %w", err)
		}
	}

	for _, e := range raw.Edges {
		if e.LengthM <= 0 {
			return nil, fmt.Errorf("%w for edge %s (got %f)", ErrInvalidEdgeLength, e.ID, e.LengthM)
		}
		if e.SpeedLimitMPS <= 0 {
			return nil, fmt.Errorf("%w for edge %s (got %f)", ErrInvalidSpeedLimit, e.ID, e.SpeedLimitMPS)
		}
		if e.Lanes <= 0 {
			return nil, fmt.Errorf("%w for edge %s (got %d)", ErrInvalidLanes, e.ID, e.Lanes)
		}
		if err := g.AddEdge(e); err != nil {
			return nil, fmt.Errorf("invalid edge %s: %w", e.ID, err)
		}
	}

	return g, nil
}

// LoadFromFile reads a graph JSON file from disk.
func LoadFromFile(filePath string) (*Graph, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open graph file: %w", err)
	}
	defer f.Close()

	return LoadFromJSON(f)
}
