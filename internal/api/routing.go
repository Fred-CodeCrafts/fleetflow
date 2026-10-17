package api

import (
	"net/http"

	"fleetflow/internal/domain"
)

// Error codes for a route query the endpoint refuses.
const (
	codeFromNodeRequired = "from_node_required"
	codeToNodeRequired   = "to_node_required"
)

// routeRequest is the body of POST /api/route (DESIGN 3.5).
type routeRequest struct {
	FromNode string `json:"from_node"`
	ToNode   string `json:"to_node"`
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
}

// handleRoute answers a route query from the engine's active snapshot. A query
// loads one immutable snapshot and computes on it, so it never waits on a report
// or a customization, and never touches PostgreSQL (DESIGN 3.5).
func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	if s.eng == nil {
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

	// An unknown node and an unreachable pair come back as the engine's own
	// sentinels; statusFor and writeError turn them into a status and a body that
	// name what went wrong.
	path, version, err := s.eng.RouteWithVersion(req.FromNode, req.ToNode)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, routeResponse{
		Edges:         s.edgeIDs(path.Edges),
		CostS:         path.CostS,
		MetricVersion: version,
	})
}

// edgeIDs translates the edge indices a Path carries into the public edge IDs.
// The result is never nil, so the empty path between a node and itself encodes as
// [] rather than null.
//
// The indices belong to the graph the engine was built over, which WithEngine
// requires to be this server's graph.
func (s *Server) edgeIDs(indices []int) []string {
	ids := make([]string, 0, len(indices))
	for _, i := range indices {
		ids = append(ids, s.graph.Edges[i].ID)
	}
	return ids
}
