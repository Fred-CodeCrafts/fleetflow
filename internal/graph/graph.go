package graph

import (
	"errors"
	"fmt"
	"math"
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

// Point is one vertex of an edge's drawn geometry, in latitude and longitude.
//
// It exists because a road is not a straight line between two junctions. A
// hand-written graph can draw node to node and carry no shape; a network
// imported from a source file carries the vertices that file gave, so its roads
// bend where the real ones bend.
type Point struct {
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

	// Shape is the drawn centreline, when the source had one. Empty means
	// "straight from From's node to To's node", which is what the grid is.
	//
	// This makes Edge non-comparable with ==, which is why nothing in this
	// package compares two edges as values; lookups go through edgeIndices.
	Shape []Point `json:"shape,omitempty"`

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

	// The index-addressed view of the same topology. A node's index is its
	// insertion order and matches the value in nodeIndices; nodeIDs is the
	// reverse mapping. edgeFrom and edgeTo hold the endpoint indices of each
	// edge, parallel to Edges.
	//
	// It exists because routing visits nodes tens of thousands of times a step
	// and a string-keyed map lookup dominated that: on the Jakarta network,
	// replacing the maps in the Dijkstra hot loop with these slices is what
	// makes a 60,000-vehicle run keep up with its clock.
	nodeIDs  []string
	edgeFrom []int
	edgeTo   []int
	// outEdgesByIndex is outEdges keyed by node index instead of node ID.
	outEdgesByIndex [][]int
	// nodeLat and nodeLon are the coordinates by node index, which the router's
	// distance estimate reads without a map lookup.
	nodeLat []float64
	nodeLon []float64
	// nodeUnit holds each node's position as a unit vector in three dimensions,
	// three values per node. The router's estimate is a chord length between two
	// of them, which needs no trigonometry and is never longer than the
	// great-circle arc, so it stays a lower bound.
	nodeUnit []float64
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

// NodeIndex returns the index of a node ID, and whether it exists.
func (g *Graph) NodeIndex(id string) (int, bool) {
	idx, ok := g.nodeIndices[id]
	return idx, ok
}

// NodeCount reports how many nodes the graph has. Node indices are in
// [0, NodeCount()).
func (g *Graph) NodeCount() int { return len(g.nodeIDs) }

// NodeIDAt returns the node ID at an index. The index must be in range.
func (g *Graph) NodeIDAt(i int) string { return g.nodeIDs[i] }

// NodeLatLonAt returns the coordinates of a node, addressed by index.
func (g *Graph) NodeLatLonAt(i int) (lat, lon float64) { return g.nodeLat[i], g.nodeLon[i] }

// ChordMetres returns the straight-line distance between two nodes through the
// earth, in metres, addressed by node index.
//
// It is a lower bound on the great-circle distance between them — a chord is
// never longer than its arc — and it needs no trigonometry, which matters
// because the router evaluates it for every node it expands. Dividing it by the
// earth's radius gives the central angle, which is the same quantity
// MetresBetween returns, only understated.
func (g *Graph) ChordMetres(a, b int) float64 {
	const earthRadiusM = 6371000.0
	ax, ay, az := g.nodeUnit[a*3], g.nodeUnit[a*3+1], g.nodeUnit[a*3+2]
	bx, by, bz := g.nodeUnit[b*3], g.nodeUnit[b*3+1], g.nodeUnit[b*3+2]
	dx, dy, dz := ax-bx, ay-by, az-bz
	return earthRadiusM * math.Sqrt(dx*dx+dy*dy+dz*dz)
}

// MaxSpeedMPS returns the fastest speed limit any edge in the graph allows, or 0
// for a graph with no edges.
func (g *Graph) MaxSpeedMPS() float64 {
	var max float64
	for _, e := range g.Edges {
		if e.SpeedLimitMPS > max {
			max = e.SpeedLimitMPS
		}
	}
	return max
}

// MetresBetween is the great-circle distance between two points, in metres.
//
// The haversine form is used because the plain spherical law of cosines loses
// precision at the short distances between junctions.
func MetresBetween(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusM = 6371000.0
	radians := math.Pi / 180
	dLat := (lat2 - lat1) * radians
	dLon := (lon2 - lon1) * radians
	sinLat := math.Sin(dLat / 2)
	sinLon := math.Sin(dLon / 2)
	a := sinLat*sinLat + math.Cos(lat1*radians)*math.Cos(lat2*radians)*sinLon*sinLon
	if a <= 0 {
		return 0
	}
	return 2 * earthRadiusM * math.Asin(math.Sqrt(a))
}

// GeometrySlack reports how much further apart two junctions can be on the map
// than their connecting road is long.
//
// It exists because a router's distance estimate has to be a lower bound, and
// straight-line distance is only a lower bound on travel distance when the
// coordinates and the road lengths agree. On a real network they do, and this is
// a little over 1. On a hand-written fixture they may not: test_graph.json has a
// 600 m edge between junctions 2.2 km apart, so trusting the coordinates there
// would make the estimate overstate the remaining cost and the search would
// return a route that is not the cheapest.
//
// It is the largest such ratio over the graph's edges, so dividing the
// straight-line distance by it can only ever understate. The cost of being
// pessimistic is a slightly wider search, never a wrong answer.
func (g *Graph) GeometrySlack() float64 {
	slack := 1.0
	for i, e := range g.Edges {
		if e.LengthM <= 0 {
			continue
		}
		fromLat, fromLon := g.NodeLatLonAt(g.edgeFrom[i])
		toLat, toLon := g.NodeLatLonAt(g.edgeTo[i])
		if ratio := MetresBetween(fromLat, fromLon, toLat, toLon) / e.LengthM; ratio > slack {
			slack = ratio
		}
	}
	return slack
}

// EdgeFromIndex returns the index of the node an edge leaves. The edge index
// must be valid.
func (g *Graph) EdgeFromIndex(edge int) int { return g.edgeFrom[edge] }

// EdgeToIndex returns the index of the node an edge reaches. The edge index must
// be valid.
func (g *Graph) EdgeToIndex(edge int) int { return g.edgeTo[edge] }

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
	g.nodeIDs = append(g.nodeIDs, node.ID)
	g.outEdgesByIndex = append(g.outEdgesByIndex, nil)
	g.nodeLat = append(g.nodeLat, node.Lat)
	g.nodeLon = append(g.nodeLon, node.Lon)
	unit := unitVector(node.Lat, node.Lon)
	g.nodeUnit = append(g.nodeUnit, unit[0], unit[1], unit[2])
	g.Nodes[node.ID] = node
	return nil
}

// unitVector returns a latitude/longitude pair as a unit vector on the sphere.
func unitVector(lat, lon float64) [3]float64 {
	radians := math.Pi / 180
	phi, lambda := lat*radians, lon*radians
	cosPhi := math.Cos(phi)
	return [3]float64{cosPhi * math.Cos(lambda), cosPhi * math.Sin(lambda), math.Sin(phi)}
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
	// Both endpoints were checked above, so the lookups cannot fail.
	fromIdx, toIdx := g.nodeIndices[edge.From], g.nodeIndices[edge.To]
	g.edgeFrom = append(g.edgeFrom, fromIdx)
	g.edgeTo = append(g.edgeTo, toIdx)
	g.outEdgesByIndex[fromIdx] = append(g.outEdgesByIndex[fromIdx], idx)
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

// OutgoingEdgeIndicesByIndex returns the edge indices leaving a node, addressed
// by node index. It is the form the router's inner loop uses, where a map lookup
// per expansion is the difference between keeping up with the clock and not.
//
// The returned slice must not be modified.
func (g *Graph) OutgoingEdgeIndicesByIndex(node int) []int {
	return g.outEdgesByIndex[node]
}
