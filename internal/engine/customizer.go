package engine

import (
	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
)

// buildDynamicCosts reads the smoothed congestion of every edge and returns a
// fresh cost slice with the dynamic weighting applied:
//
//	cost = free_flow_s * (1 + alpha * rho^beta)
//
// states must hold one EdgeState per graph edge, in edge index order. Each edge
// is read under its own lock, so a slow edge cannot block the others. The
// returned slice is owned by the caller and is safe to publish as a Metric.
func buildDynamicCosts(g *graph.Graph, states []*weights.EdgeState, p weights.Params) []float64 {
	costs := make([]float64, len(g.Edges))
	for i := range g.Edges {
		costs[i] = weights.Cost(g.Edges[i].FreeFlowS, p.Alpha, p.Beta, states[i].Rho())
	}
	return costs
}

// buildStaticCosts returns the free-flow time of every edge and ignores all
// congestion state. This is the baseline mode: the weights are fixed, so no
// report can change a route.
func buildStaticCosts(g *graph.Graph) []float64 {
	costs := make([]float64, len(g.Edges))
	for i, e := range g.Edges {
		costs[i] = e.FreeFlowS
	}
	return costs
}
