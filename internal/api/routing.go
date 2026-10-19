package api

import (
	"net/http"

	"fleetflow/internal/domain"
	"fleetflow/internal/graph"
)

// Error codes for a route query the endpoint refuses.
const (
	codeFromNodeRequired      = "from_node_required"
	codeToNodeRequired        = "to_node_required"
	codeCurrentEdgeUnknown    = "current_edge_unknown"
	codeCurrentEdgesNotAChain = "current_edges_not_contiguous"
)

// routeRequest is the body of POST /api/route (DESIGN 3.5).
type routeRequest struct {
	FromNode string `json:"from_node"`
	ToNode   string `json:"to_node"`
	// CurrentEdges is the path the caller already holds, in order, starting at
	// from_node. It is optional: a caller planning a trip sends only from_node
	// and to_node, while one deciding whether to switch route sends the rest of
	// the path it is driving so the server can price it. An empty list is a real
	// answer — nothing is held, so the held cost is zero — and is told apart
	// from omitting the field.
	CurrentEdges []string `json:"current_edges"`
}

// routeResponse is the answer to a route query: the edges to walk in order, what
// the trip costs on the current weights, and which snapshot those weights were.
//
// The version lets a client tell whether the route it just asked for was built
// from the same weights as the one it asked for a moment ago, rather than
// guessing from the cost.
type routeResponse struct {
	Edges         []string `json:"edges"`
	CostS         float64  `json:"cost_s"`
	MetricVersion uint64   `json:"metric_version"`
	// CurrentCostS is what current_edges costs on the same snapshot as cost_s. It
	// is present only when the request carried current_edges, and is a pointer so
	// that a legitimate zero is not dropped by omitempty.
	CurrentCostS *float64 `json:"current_cost_s,omitempty"`
}

// handleRoute answers a route query from the engine's active snapshot. A query
// loads one immutable snapshot and computes on it, so it never waits on a report
// or a customization, and never touches PostgreSQL (DESIGN 3.5).
func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	g, eng := s.networkSnapshot()
	if eng == nil || g == nil {
		writeError(w, r, errNoEngine)
		return
	}

	var req routeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if req.FromNode == "" {
		writeError(w, r, domain.Invalid(codeFromNodeRequired, "from_node is required"))
		return
	}
	if req.ToNode == "" {
		writeError(w, r, domain.Invalid(codeToNodeRequired, "to_node is required"))
		return
	}

	// Resolve the held path before the engine is asked for anything, so a
	// malformed one is refused without a route being computed.
	held, err := heldEdgeIndices(g, req.FromNode, req.CurrentEdges)
	if err != nil {
		writeError(w, r, err)
		return
	}

	// An unknown node and an unreachable pair come back as the engine's own
	// sentinels; statusFor and writeError turn them into a status and a body that
	// name what went wrong.
	answer, err := eng.RouteWithHeld(req.FromNode, req.ToNode, held)
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp := routeResponse{
		Edges:         edgeIDs(g, answer.Path.Edges),
		CostS:         answer.Path.CostS,
		MetricVersion: answer.Version,
	}
	if answer.HeldPriced {
		resp.CurrentCostS = &answer.HeldCostS
	}

	writeJSON(w, http.StatusOK, resp)
}

// heldEdgeIndices resolves the edge IDs of a held path to graph indices. It
// returns nil when no held path was sent at all, so a plain route query is
// unchanged; an explicitly empty list comes back as a non-nil empty slice, which
// prices to zero and is told apart from not asking.
//
// The held path describes where the caller is going, so it must be a chain: every
// edge must exist, each must meet the next, and the first must start at the
// query's origin. Anything else is the client's bad input, refused here with a
// 400 rather than silently priced as a shorter or wrong path.
func heldEdgeIndices(g *graph.Graph, fromNode string, ids []string) ([]int, error) {
	if ids == nil {
		return nil, nil
	}

	indices := make([]int, len(ids))
	for i, id := range ids {
		_, idx, ok := g.EdgeByID(id)
		if !ok {
			return nil, domain.Invalid(codeCurrentEdgeUnknown,
				"current_edges[%d] %q is not an edge of the graph", i, id)
		}
		indices[i] = idx
	}
	if len(indices) == 0 {
		return indices, nil
	}

	if got := g.Edges[indices[0]].From; got != fromNode {
		return nil, domain.Invalid(codeCurrentEdgesNotAChain,
			"current_edges must start at from_node %q, but starts at %q", fromNode, got)
	}
	for i := 1; i < len(indices); i++ {
		if g.Edges[indices[i]].From != g.Edges[indices[i-1]].To {
			return nil, domain.Invalid(codeCurrentEdgesNotAChain,
				"current_edges[%d] does not continue from current_edges[%d]", i, i-1)
		}
	}
	return indices, nil
}

// edgeIDs translates the edge indices a Path carries into the public edge IDs.
// The result is never nil, so the empty path between a node and itself encodes as
// [] rather than null.
//
// The indices belong to the graph the engine was built over, which WithEngine
// requires to be this server's graph.
func edgeIDs(g *graph.Graph, indices []int) []string {
	ids := make([]string, 0, len(indices))
	for _, i := range indices {
		ids = append(ids, g.Edges[i].ID)
	}
	return ids
}
