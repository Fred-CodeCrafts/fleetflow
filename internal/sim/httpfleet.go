package sim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
)

// HTTPFleet is a FleetService that goes over the wire. A fleet vehicle's route
// requests and speed reports are sent to POST /api/route and POST /api/reports on
// the server's own base URL, so simulated traffic is validated, authenticated and
// measured exactly like an external client (DESIGN 3.4, PRD US-7).
//
// Every call is synchronous and every request is built from the simulation's own
// plan, so a seeded run sends the same requests in the same order and stays
// deterministic. It is deliberately not safe for concurrent use: the run loop
// drives it from one goroutine.
type HTTPFleet struct {
	baseURL string
	apiKey  string
	client  *http.Client

	// The simulator drives on edge indices while the API speaks edge IDs, so the
	// client keeps both directions of the mapping for the graph it was built for.
	edgeIndex map[string]int
	edgeIDs   []string
}

// NewHTTPFleet returns a client for the API at baseURL, authenticating with
// apiKey. g must be the same graph the server routes over, because a route reply
// carries edge IDs that have to be turned back into the indices the simulator
// drives on.
//
// A nil client uses http.DefaultClient; production passes one with a timeout, so
// a stalled server cannot hold the simulation.
func NewHTTPFleet(baseURL, apiKey string, g *graph.Graph, client *http.Client) *HTTPFleet {
	if client == nil {
		client = http.DefaultClient
	}

	f := &HTTPFleet{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		client:    client,
		edgeIndex: make(map[string]int, len(g.Edges)),
		edgeIDs:   make([]string, len(g.Edges)),
	}
	for i, e := range g.Edges {
		f.edgeIndex[e.ID] = i
		f.edgeIDs[i] = e.ID
	}
	return f
}

// routeQuery is the body of POST /api/route. current_edges is always written —
// null when the caller holds nothing — so the server can tell "no held path" from
// "an empty one", which the API distinguishes.
type routeQuery struct {
	FromNode     string   `json:"from_node"`
	ToNode       string   `json:"to_node"`
	CurrentEdges []string `json:"current_edges"`
}

// routeReply is the answer to POST /api/route.
type routeReply struct {
	Edges         []string `json:"edges"`
	CostS         float64  `json:"cost_s"`
	MetricVersion uint64   `json:"metric_version"`
	CurrentCostS  *float64 `json:"current_cost_s"`
}

// Route implements FleetService over POST /api/route. When held is non-nil the
// reply prices it, on the same snapshot as the plan, exactly as the in-process
// engine does.
func (f *HTTPFleet) Route(fromID, toID string, held []int) (engine.RouteAnswer, error) {
	query := routeQuery{FromNode: fromID, ToNode: toID}
	if held != nil {
		ids, err := f.edgeIDsFor(held)
		if err != nil {
			return engine.RouteAnswer{}, err
		}
		query.CurrentEdges = ids
	}

	var reply routeReply
	if err := f.post("/api/route", query, &reply); err != nil {
		return engine.RouteAnswer{}, err
	}

	edges, err := f.indicesFor(reply.Edges)
	if err != nil {
		return engine.RouteAnswer{}, err
	}

	answer := engine.RouteAnswer{
		Path:    &engine.Path{Edges: edges, CostS: reply.CostS},
		Version: reply.MetricVersion,
	}
	if reply.CurrentCostS != nil {
		answer.HeldCostS = *reply.CurrentCostS
		answer.HeldPriced = true
	}
	return answer, nil
}

// reportRequest is the body of POST /api/reports.
type reportRequest struct {
	VehicleID string  `json:"vehicle_id"`
	EdgeID    string  `json:"edge_id"`
	SpeedMPS  float64 `json:"speed_mps"`
	TS        *int64  `json:"ts,omitempty"`
}

// Report implements FleetService over POST /api/reports. The observation time is
// sent, rather than left to the server's wall clock, so the smoothing reflects
// simulated time and a replay of the same seed folds in the same values.
func (f *HTTPFleet) Report(vehicleID, edgeID string, speedMPS float64, now time.Time) error {
	ms := now.UnixMilli()
	return f.post("/api/reports", reportRequest{
		VehicleID: vehicleID,
		EdgeID:    edgeID,
		SpeedMPS:  speedMPS,
		TS:        &ms,
	}, nil)
}

// batchRequest is the body of POST /api/reports/batch.
type batchRequest struct {
	Observations []reportRequest `json:"observations"`
}

// ReportBatch implements BatchReporter over POST /api/reports/batch.
//
// One request carries a whole step's observations, which is what makes a large
// run usable: per-vehicle reporting put 30,000 serial round trips in one step,
// and the run advanced two simulated seconds in forty seconds of wall clock.
//
// The observations are sent with their shared timestamp and the server folds them
// in order, so the result is the same as sending them one at a time.
func (f *HTTPFleet) ReportBatch(obs []SpeedObservation, now time.Time) error {
	ms := now.UnixMilli()
	req := batchRequest{Observations: make([]reportRequest, len(obs))}
	for i, o := range obs {
		req.Observations[i] = reportRequest{
			VehicleID: o.VehicleID,
			EdgeID:    o.EdgeID,
			SpeedMPS:  o.SpeedMPS,
			TS:        &ms,
		}
	}
	return f.post("/api/reports/batch", req, nil)
}

// edgeIDsFor turns edge indices into the IDs the API speaks. A non-nil empty
// slice of indices stays a non-nil empty slice of IDs, which encodes as [] and
// tells the server an empty path was held rather than none at all.
func (f *HTTPFleet) edgeIDsFor(indices []int) ([]string, error) {
	ids := make([]string, len(indices))
	for i, idx := range indices {
		if idx < 0 || idx >= len(f.edgeIDs) {
			return nil, fmt.Errorf("edge index %d is outside the graph", idx)
		}
		ids[i] = f.edgeIDs[idx]
	}
	return ids, nil
}

// indicesFor turns the edge IDs a route reply carries back into graph indices. A
// reply naming an edge the client's graph does not have means the two disagree,
// which is a mistake worth stopping on rather than routing a vehicle onto it.
func (f *HTTPFleet) indicesFor(ids []string) ([]int, error) {
	indices := make([]int, len(ids))
	for i, id := range ids {
		idx, ok := f.edgeIndex[id]
		if !ok {
			return nil, fmt.Errorf("route reply names edge %q, which is not in the graph", id)
		}
		indices[i] = idx
	}
	return indices, nil
}

// post sends one JSON request and, when out is non-nil, decodes the JSON reply
// into it. A response that is not 2xx becomes an error carrying the API's own
// code and message, so a caller sees exactly what an external client would.
func (f *HTTPFleet) post(path string, body, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", path, err)
	}

	req, err := http.NewRequest(http.MethodPost, f.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("build %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.apiKey)

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("call %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return refused(path, resp)
	}
	if out == nil {
		// Drain the body so the connection can be reused by the next report.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s reply: %w", path, err)
	}
	return nil
}

// apiError is a request the API refused. It keeps the status, code and message
// the server sent, so a caller can act on the same information an external client
// would receive.
type apiError struct {
	status  int
	code    string
	message string
}

// Error implements the error interface.
func (e *apiError) Error() string {
	return fmt.Sprintf("api returned %d %s: %s", e.status, e.code, e.message)
}

// refused builds the error for a non-2xx response from the API's own error body.
// A body that is not that shape still leaves the status to report.
func refused(path string, resp *http.Response) error {
	// The API bounds every error body, and so does this read: a proxy in front of
	// the service could answer with anything.
	const maxErrorBytes = 4096

	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, maxErrorBytes)).Decode(&body)
	if body.Code == "" {
		body.Code = "unexpected_status"
		body.Message = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf("%s: %w", path, &apiError{status: resp.StatusCode, code: body.Code, message: body.Message})
}
