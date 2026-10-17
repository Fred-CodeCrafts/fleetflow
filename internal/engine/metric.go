// Package engine turns the mutable per-edge congestion state into immutable
// routing snapshots and answers route queries against them. Queries only ever
// read a published snapshot, so they never block on reports or customization.
package engine

// Metric is an immutable snapshot of the cost of every edge in the graph. It is
// built once, published as a whole, and never modified afterwards, so any
// number of route queries can read one concurrently without locks.
type Metric struct {
	costs   []float64
	version uint64
}

// NewMetric wraps costs as a metric. The slice is owned by the Metric
// afterwards: the caller must not keep a reference to it or change it.
func NewMetric(costs []float64, version uint64) *Metric {
	return &Metric{costs: costs, version: version}
}

// Version reports the monotonically increasing snapshot number.
func (m *Metric) Version() uint64 { return m.version }

// Len reports how many edges the snapshot covers. It matches the length of the
// graph's edge slice.
func (m *Metric) Len() int { return len(m.costs) }

// Cost reports the cost in seconds of the edge at index i. i must be a valid
// edge index in the graph the snapshot was built for.
func (m *Metric) Cost(i int) float64 { return m.costs[i] }
