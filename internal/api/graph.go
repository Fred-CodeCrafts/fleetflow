package api

import (
	"net/http"
	"sort"
)

// graphResponse is the body of GET /api/graph: the road network itself, so the
// dashboard can draw the map from the same edge indices and coordinates the
// engine routes over, with no map tiles and no second source of geometry (T49).
//
// The graph does not change while the server runs, so the response carries a
// cache header; the congestion drawn on top of it arrives separately over the
// event stream, because that part changes every second.
type graphResponse struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

// graphNode is one place on the map.
type graphNode struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// graphEdge is one road segment. FreeFlowS and Capacity are the derived values
// the speed model uses, sent so that a client colouring by congestion can say
// what it is colouring against.
type graphEdge struct {
	ID            string  `json:"id"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	LengthM       float64 `json:"length_m"`
	Lanes         int     `json:"lanes"`
	SpeedLimitMPS float64 `json:"speed_limit_mps"`
	FreeFlowS     float64 `json:"free_flow_s"`
	Capacity      float64 `json:"capacity"`
}

// handleGraph serves the road network. It is public for the same reason the
// metrics are: the dashboard reads it, and it discloses nothing the map does not
// already draw.
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	body := graphResponse{Nodes: make([]graphNode, 0, len(s.graph.Nodes)), Edges: make([]graphEdge, 0, len(s.graph.Edges))}

	for _, node := range s.graph.Nodes {
		body.Nodes = append(body.Nodes, graphNode{ID: node.ID, Lat: node.Lat, Lon: node.Lon})
	}
	// A map iterates in no order at all, and a client that diffs two responses
	// should not be told the graph changed.
	sort.Slice(body.Nodes, func(i, j int) bool { return body.Nodes[i].ID < body.Nodes[j].ID })

	for _, edge := range s.graph.Edges {
		body.Edges = append(body.Edges, graphEdge{
			ID:            edge.ID,
			From:          edge.From,
			To:            edge.To,
			LengthM:       edge.LengthM,
			Lanes:         edge.Lanes,
			SpeedLimitMPS: edge.SpeedLimitMPS,
			FreeFlowS:     edge.FreeFlowS,
			Capacity:      edge.Capacity,
		})
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, body)
}
