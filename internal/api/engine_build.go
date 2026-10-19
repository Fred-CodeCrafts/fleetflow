package api

import (
	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
)

// newEngineFor builds the dynamic engine a server routes with over g.
//
// It exists so that the graph and its engine are always built by the same code:
// the server's startup path and its network-import path both call this, so an
// imported network is routed exactly as the configured one was. A second
// construction site would be a second place for the parameters to drift.
func newEngineFor(g *graph.Graph) (*engine.Engine, error) {
	return engine.New(g, engine.NewDijkstraRouter(g), weights.DefaultParams())
}

// engineForBuilds is the seam the server uses when it needs an engine. It is a
// variable rather than a direct call so tests can substitute a construction that
// fails, which is the only way to exercise the error path of installGraph
// without corrupting a real graph.
var engineForBuilds = newEngineFor
