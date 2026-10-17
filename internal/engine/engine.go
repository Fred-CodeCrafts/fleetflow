package engine

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
)

var (
	// ErrEdgeNotFound is returned when a report names an edge missing from the graph.
	ErrEdgeNotFound = errors.New("edge not found")
	// ErrNoMetric is returned when a query runs before any metric was published.
	ErrNoMetric = errors.New("no metric published")
)

// Engine holds the live congestion state of one routing mode and answers route
// queries against the most recently published metric snapshot.
//
// Reports and customization touch per-edge state, while queries only load the
// active snapshot pointer. A query therefore never waits on either, and never
// sees a half-built metric.
type Engine struct {
	graph  *graph.Graph
	router Router
	params weights.Params

	states    []*weights.EdgeState
	edgeIndex map[string]int

	// static selects baseline mode: the metric is fixed to free-flow times and
	// reports are ignored.
	static bool

	active  atomic.Pointer[Metric]
	version atomic.Uint64
}

// New creates an engine with dynamic weights over g. The router is required and
// the parameters must be valid; the engine publishes its first, free-flow metric
// before returning.
func New(g *graph.Graph, router Router, params weights.Params) (*Engine, error) {
	return newEngine(g, router, params, false)
}

// NewBaseline creates an engine in baseline mode: the metric is fixed to
// free-flow times and reports are accepted but ignored, so it always returns
// static shortest paths. It is the comparison arm for the dynamic engine.
func NewBaseline(g *graph.Graph, router Router) (*Engine, error) {
	return newEngine(g, router, weights.DefaultParams(), true)
}

func newEngine(g *graph.Graph, router Router, params weights.Params, static bool) (*Engine, error) {
	if g == nil {
		return nil, errors.New("graph is required")
	}
	if router == nil {
		return nil, errors.New("router is required")
	}
	if err := params.Validate(); err != nil {
		return nil, err
	}

	e := &Engine{
		graph:     g,
		router:    router,
		params:    params,
		static:    static,
		edgeIndex: make(map[string]int, len(g.Edges)),
	}
	for i, edge := range g.Edges {
		e.edgeIndex[edge.ID] = i
	}

	if static {
		e.publish(buildStaticCosts(g))
		return e, nil
	}

	e.states = make([]*weights.EdgeState, len(g.Edges))
	for i := range e.states {
		e.states[i] = weights.NewEdgeState()
	}
	e.publish(buildDynamicCosts(g, e.states, params))

	return e, nil
}

// Params returns the parameters the engine was built with.
func (e *Engine) Params() weights.Params { return e.params }

// ActiveMetric returns the snapshot the next query will use.
func (e *Engine) ActiveMetric() *Metric { return e.active.Load() }

// Report folds one vehicle report into the congestion state of an edge. The
// free-flow speed used to turn the observed speed into congestion comes from the
// edge's speed limit.
func (e *Engine) Report(edgeID string, speedMPS float64, now time.Time) error {
	i, ok := e.edgeIndex[edgeID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrEdgeNotFound, edgeID)
	}
	if e.static {
		return nil // baseline mode has no weights to update
	}
	e.states[i].Observe(speedMPS, e.graph.Edges[i].SpeedLimitMPS, e.params.Gamma, now)
	return nil
}

// Route answers a route query from the currently published snapshot. The pointer
// is loaded exactly once, so the whole query runs on one immutable metric even if
// a new snapshot is published mid-query.
func (e *Engine) Route(fromID, toID string) (*Path, error) {
	m := e.active.Load()
	if m == nil {
		return nil, ErrNoMetric
	}
	return e.router.Route(m, fromID, toID)
}

// Customize rebuilds the metric from the current edge states and publishes it
// atomically. Older snapshots stay valid until no query references them. In
// baseline mode the metric never changes, so the active snapshot is returned
// unchanged.
func (e *Engine) Customize() *Metric {
	if e.static {
		return e.active.Load()
	}
	return e.publish(buildDynamicCosts(e.graph, e.states, e.params))
}

// publish stores a freshly built cost slice as the active snapshot and bumps the
// version.
func (e *Engine) publish(costs []float64) *Metric {
	m := NewMetric(costs, e.version.Add(1))
	e.active.Store(m)
	return m
}
