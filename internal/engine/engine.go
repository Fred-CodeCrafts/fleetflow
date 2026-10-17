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

	active  atomic.Pointer[Metric]
	version atomic.Uint64
}

// New creates an engine with dynamic weights over g. The router is required and
// the parameters must be valid; the engine publishes its first, free-flow metric
// before returning.
func New(g *graph.Graph, router Router, params weights.Params) (*Engine, error) {
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
		states:    make([]*weights.EdgeState, len(g.Edges)),
		edgeIndex: make(map[string]int, len(g.Edges)),
	}
	for i, edge := range g.Edges {
		e.states[i] = weights.NewEdgeState()
		e.edgeIndex[edge.ID] = i
	}

	e.Customize()
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
// atomically. Older snapshots stay valid until no query references them.
func (e *Engine) Customize() *Metric {
	m := NewMetric(buildDynamicCosts(e.graph, e.states, e.params), e.version.Add(1))
	e.active.Store(m)
	return m
}
