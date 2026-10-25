package engine

import "fleetflow/internal/graph"

// Router answers route queries against an immutable metric. The road topology is
// fixed for the lifetime of a Router, so it is captured at construction time and
// the graph is not passed on every query.
//
// Callers depend on this interface, not on Dijkstra, so a faster algorithm such
// as Customizable Contraction Hierarchies can replace it later without changing
// the simulator or the HTTP API.
type Router interface {
	Route(m *Metric, fromID, toID string) (*Path, error)
}

// dijkstraRouter adapts the Dijkstra implementation to Router.
type dijkstraRouter struct {
	g *graph.Graph
}

// NewDijkstraRouter returns a Router that finds least-cost paths with Dijkstra's
// algorithm over a binary heap.
func NewDijkstraRouter(g *graph.Graph) Router {
	return &dijkstraRouter{g: g}
}

// Route implements Router.
func (r *dijkstraRouter) Route(m *Metric, fromID, toID string) (*Path, error) {
	return dijkstraRoute(r.g, m, fromID, toID)
}
