package graph

import (
	"errors"
	"fmt"
)

var (
	ErrDuplicateNode = errors.New("duplicate node ID")
	ErrDuplicateEdge = errors.New("duplicate edge ID")
	ErrNodeNotFound  = errors.New("node not found")
)

// Node represents a vertex in the road graph with geographical coordinates.
type Node struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Edge represents a directed road segment connecting two nodes.
type Edge struct {
	ID            string  `json:"id"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	LengthM       float64 `json:"length_m"`
	SpeedLimitMPS float64 `json:"speed_limit_mps"`
	Lanes         int     `json:"lanes"`

	// Derived values computed when the graph is loaded.
	FreeFlowS float64 `json:"free_flow_s,omitempty"`
	Capacity  float64 `json:"capacity,omitempty"`
}

// Graph holds the fixed road network topology.
type Graph struct {
	Nodes map[string]Node
	Edges []Edge

	nodeIndices map[string]int
	edgeIndices map[string]int
	outEdges    map[string][]int // from node ID -> indices in Edges
}

// New creates an empty Graph.
func New() *Graph {
	return &Graph{
		Nodes:       make(map[string]Node),
		Edges:       make([]Edge, 0),
		nodeIndices: make(map[string]int),
		edgeIndices: make(map[string]int),
		outEdges:    make(map[string][]int),
	}
}

// DefaultVehicleSpacingMeters is the standard vehicle length plus gap (7.5 meters).
const DefaultVehicleSpacingMeters = 7.5

// DeriveEdgeValues computes free-flow travel time (s) and edge vehicle capacity.
func DeriveEdgeValues(e *Edge) {
	if e.SpeedLimitMPS > 0 {
		e.FreeFlowS = e.LengthM / e.SpeedLimitMPS
	}
	e.Capacity = float64(e.Lanes) * e.LengthM / DefaultVehicleSpacingMeters
}

// AddNode adds a new node to the graph.
func (g *Graph) AddNode(node Node) error {
	if node.ID == "" {
		return errors.New("node ID cannot be empty")
	}
	if _, exists := g.Nodes[node.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateNode, node.ID)
	}
	g.nodeIndices[node.ID] = len(g.Nodes)
	g.Nodes[node.ID] = node
	return nil
}

// AddEdge adds a directed edge to the graph. Both From and To nodes must exist.
func (g *Graph) AddEdge(edge Edge) error {
	if edge.ID == "" {
		return errors.New("edge ID cannot be empty")
	}
	if _, exists := g.edgeIndices[edge.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateEdge, edge.ID)
	}
	if _, exists := g.Nodes[edge.From]; !exists {
		return fmt.Errorf("%w: from %s", ErrNodeNotFound, edge.From)
	}
	if _, exists := g.Nodes[edge.To]; !exists {
		return fmt.Errorf("%w: to %s", ErrNodeNotFound, edge.To)
	}

	DeriveEdgeValues(&edge)

	idx := len(g.Edges)
	g.Edges = append(g.Edges, edge)
	g.edgeIndices[edge.ID] = idx
	g.outEdges[edge.From] = append(g.outEdges[edge.From], idx)
	return nil
}

// EdgeByID retrieves an edge and its index by edge ID.
func (g *Graph) EdgeByID(id string) (Edge, int, bool) {
	idx, exists := g.edgeIndices[id]
	if !exists {
		return Edge{}, -1, false
	}
	return g.Edges[idx], idx, true
}

// OutgoingEdges returns all directed edges originating from a node ID.
func (g *Graph) OutgoingEdges(nodeID string) []Edge {
	indices := g.outEdges[nodeID]
	result := make([]Edge, len(indices))
	for i, idx := range indices {
		result[i] = g.Edges[idx]
	}
	return result
}

// OutgoingEdgeIndices returns the indices in g.Edges of all edges originating from nodeID.
func (g *Graph) OutgoingEdgeIndices(nodeID string) []int {
	return g.outEdges[nodeID]
}
