// Package engine turns the mutable per-edge congestion state into immutable
// routing snapshots and answers route queries against them. Queries only ever
// read a published snapshot, so they never block on reports or customization.
package engine

import "fleetflow/internal/graph"

// Metric is an immutable snapshot of the cost of every edge in the graph. It is
// built once, published as a whole, and never modified afterwards, so any
// number of route queries can read one concurrently without locks.
type Metric struct {
	costs   []float64
	version uint64
	// maxSpeedMPS is the fastest speed any edge in the graph allows, used by the
	// router's distance estimate. Zero means unknown, and the estimate then
	// contributes nothing, which degrades to plain Dijkstra rather than to a
	// wrong answer.
	maxSpeedMPS float64
	// geometrySlack is how far the graph's coordinates can overstate its road
	// lengths; see graph.GeometrySlack.
	geometrySlack float64
}

// NewMetric wraps costs as a metric. The slice is owned by the Metric
// afterwards: the caller must not keep a reference to it or change it.
//
// The fastest edge speed is carried alongside the costs because the router's
// distance estimate needs it, and a snapshot that does not know it can only fall
// back to plain Dijkstra. Prefer NewMetricFor, which takes it from the graph.
func NewMetric(costs []float64, version uint64) *Metric {
	return &Metric{costs: costs, version: version}
}

// NewMetricFor wraps costs as a metric for g, recording the graph's fastest edge
// speed and coordinate slack so route queries can use a distance estimate.
func NewMetricFor(g *graph.Graph, costs []float64, version uint64) *Metric {
	return &Metric{
		costs:         costs,
		version:       version,
		maxSpeedMPS:   g.MaxSpeedMPS(),
		geometrySlack: g.GeometrySlack(),
	}
}

// Version reports the monotonically increasing snapshot number.
func (m *Metric) Version() uint64 { return m.version }

// Len reports how many edges the snapshot covers. It matches the length of the
// graph's edge slice.
func (m *Metric) Len() int { return len(m.costs) }

// Cost reports the cost in seconds of the edge at index i. i must be a valid
// edge index in the graph the snapshot was built for.
func (m *Metric) Cost(i int) float64 { return m.costs[i] }

// MaxSpeed reports the fastest edge speed the snapshot was built for, which is
// the denominator of the router's distance estimate. Zero means unknown.
func (m *Metric) MaxSpeed() float64 { return m.maxSpeedMPS }

// GeometrySlack reports how far the graph's coordinates can overstate its roads.
// One or less means the estimate can be trusted as given.
func (m *Metric) GeometrySlack() float64 {
	if m.geometrySlack <= 0 {
		return 1
	}
	return m.geometrySlack
}
