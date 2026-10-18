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

	// Engineering counters, readable without locks by the metrics endpoint.
	lastCustomizeNanos atomic.Int64 // unix nanos of the last publish
	customizeNanos     atomic.Int64 // duration of the last customization pass
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
		e.publish(buildStaticCosts(g), 0)
		return e, nil
	}

	e.states = make([]*weights.EdgeState, len(g.Edges))
	for i := range e.states {
		e.states[i] = weights.NewEdgeState()
	}
	e.publish(buildDynamicCosts(g, e.states, params), 0)

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
	path, _, err := e.RouteWithVersion(fromID, toID)
	return path, err
}

// RouteAnswer is a route query answered from a single metric snapshot: the
// least-cost path, and, when the caller sent the path it already holds, what
// that path costs on the same snapshot.
//
// Answering both from one snapshot is the point. A caller compares the two
// costs to decide whether switching route is worth it, so a refresh landing
// between a route query and a separate price query would make the comparison
// meaningless.
type RouteAnswer struct {
	Path *Path
	// HeldCostS is what the edges the caller already holds cost on the same
	// snapshot as Path. It is only meaningful when HeldPriced is true.
	HeldCostS  float64
	HeldPriced bool
	// Version identifies the snapshot both costs came from.
	Version uint64
}

// RouteWithVersion is Route, and also reports the version of the snapshot that
// answered the query. The pointer is loaded exactly once, so the version always
// belongs to the metric the path was computed from — not to whatever happens to
// be active by the time a caller writes its response.
//
// A caller that quotes the version it used needs this: the API returns
// metric_version so a client can tell whether a route predates a refresh.
func (e *Engine) RouteWithVersion(fromID, toID string) (*Path, uint64, error) {
	answer, err := e.RouteWithHeld(fromID, toID, nil)
	if err != nil {
		return nil, 0, err
	}
	return answer.Path, answer.Version, nil
}

// RouteWithHeld answers a route query and, when held is non-nil, prices those
// edges on the very snapshot the path was computed from. held holds edge indices
// into the graph the engine was built over; an index outside the graph is
// ErrEdgeNotFound.
//
// The active pointer is loaded exactly once, so the path, the held cost and the
// version all belong to one immutable metric even if a refresh lands mid-query.
func (e *Engine) RouteWithHeld(fromID, toID string, held []int) (RouteAnswer, error) {
	m := e.active.Load()
	if m == nil {
		return RouteAnswer{}, ErrNoMetric
	}

	path, err := e.router.Route(m, fromID, toID)
	if err != nil {
		return RouteAnswer{}, err
	}

	answer := RouteAnswer{Path: path, Version: m.Version()}
	if held != nil {
		for _, i := range held {
			if i < 0 || i >= m.Len() {
				return RouteAnswer{}, fmt.Errorf("%w: edge index %d", ErrEdgeNotFound, i)
			}
			answer.HeldCostS += m.Cost(i)
		}
		answer.HeldPriced = true
	}
	return answer, nil
}

// PathCost returns the cost of a sequence of edge indices on the active metric.
// The snapshot is loaded once, so the whole sum is internally consistent. The
// second result is false when no metric is published or an index is out of
// range.
func (e *Engine) PathCost(edges []int) (float64, bool) {
	m := e.active.Load()
	if m == nil {
		return 0, false
	}

	var total float64
	for _, i := range edges {
		if i < 0 || i >= m.Len() {
			return 0, false
		}
		total += m.Cost(i)
	}
	return total, true
}

// Customize rebuilds the metric from the current edge states and publishes it
// atomically. Older snapshots stay valid until no query references them. In
// baseline mode the metric never changes, so the active snapshot is returned
// unchanged.
func (e *Engine) Customize() *Metric {
	if e.static {
		return e.active.Load()
	}
	start := time.Now()
	costs := buildDynamicCosts(e.graph, e.states, e.params)
	return e.publish(costs, time.Since(start))
}

// publish stores a freshly built cost slice as the active snapshot, bumps the
// version and records when the pass happened and how long it took.
func (e *Engine) publish(costs []float64, took time.Duration) *Metric {
	m := NewMetric(costs, e.version.Add(1))
	e.active.Store(m)
	e.lastCustomizeNanos.Store(time.Now().UnixNano())
	e.customizeNanos.Store(int64(took))
	return m
}

// LastCustomize reports when the active metric was last published.
func (e *Engine) LastCustomize() time.Time {
	return time.Unix(0, e.lastCustomizeNanos.Load())
}

// CustomizeDuration reports how long the most recent customization pass took.
func (e *Engine) CustomizeDuration() time.Duration {
	return time.Duration(e.customizeNanos.Load())
}

// MetricAge reports how old the active metric is at time now. Route queries use
// the last published snapshot, so this is the staleness of their weights.
func (e *Engine) MetricAge(now time.Time) time.Duration {
	last := e.LastCustomize()
	if last.IsZero() || now.Before(last) {
		return 0
	}
	return now.Sub(last)
}
