package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
)

// newEngine builds the live routing engine a test server serves from.
func newEngine(t *testing.T, g *graph.Graph) *engine.Engine {
	t.Helper()

	e, err := engine.New(g, engine.NewDijkstraRouter(g), weights.DefaultParams())
	if err != nil {
		t.Fatalf("failed to build engine: %v", err)
	}
	return e
}

// postJSON sends body to path, authenticated as the suite's key, and returns the
// recorder the server wrote to. The ingest endpoints require a key, so a helper
// that could not present one would only ever exercise the 401 path; postJSONAs is
// for the tests that mean to present a particular one.
func postJSON(t *testing.T, s *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIKey)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestReport_AcceptsAValidReport(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	rec := postJSON(t, s, "/api/reports", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":13.9}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body)
	}
	if got := decodeBody(t, rec)["status"]; got != "accepted" {
		t.Errorf("status = %v, want accepted", got)
	}
}

// TestReport_FeedsTheWeightStore is the T37 done-when in a test: a report has to
// reach the weights the routes are built from. A crawl on one edge must make that
// edge cost more after the next customization.
func TestReport_FeedsTheWeightStore(t *testing.T) {
	g := testGraph(t)
	e := newEngine(t, g)
	s := testServer(t, WithEngine(e))

	_, index, ok := g.EdgeByID("e_AB")
	if !ok {
		t.Fatal("the fixture graph has no e_AB edge")
	}
	before := e.ActiveMetric().Cost(index)

	rec := postJSON(t, s, "/api/reports", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":1}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body)
	}

	e.Customize()
	if after := e.ActiveMetric().Cost(index); after <= before {
		t.Errorf("edge e_AB costs %v after a crawl report, want more than %v", after, before)
	}
}

func TestReport_AcceptsATimestamp(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	rec := postJSON(t, s, "/api/reports",
		`{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":10,"ts":1700000000000}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body)
	}
}

func TestReport_RejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{"missing vehicle", `{"edge_id":"e_AB","speed_mps":10}`, codeVehicleIDRequired},
		{"missing edge", `{"vehicle_id":"v1","speed_mps":10}`, codeEdgeIDRequired},
		{"zero speed", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":0}`, codeSpeedOutOfRange},
		{"negative speed", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":-1}`, codeSpeedOutOfRange},
		{"implausible speed", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":900}`, codeSpeedOutOfRange},
		{"zero timestamp", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":10,"ts":0}`, codeTimestampInvalid},
		{"negative timestamp", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":10,"ts":-5}`, codeTimestampInvalid},
	}

	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postJSON(t, s, "/api/reports", tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
			if got := decodeBody(t, rec)["code"]; got != tt.code {
				t.Errorf("code = %v, want %s", got, tt.code)
			}
		})
	}
}

// TestReport_UnknownEdgeIsAClearNotFound guards the translation of the routing
// core's sentinel: the client must be told which edge is unknown, not handed a
// generic internal failure.
func TestReport_UnknownEdgeIsAClearNotFound(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	rec := postJSON(t, s, "/api/reports", `{"vehicle_id":"v1","edge_id":"nope","speed_mps":10}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}

	body := decodeBody(t, rec)
	if body["code"] != "edge_not_found" {
		t.Errorf("code = %v, want edge_not_found", body["code"])
	}
	if body["message"] == "internal server error" {
		t.Error("the client was handed a generic internal failure")
	}
}

func TestReport_WithoutAnEngineIsUnavailable(t *testing.T) {
	s := testServer(t)

	rec := postJSON(t, s, "/api/reports", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":10}`)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if got := decodeBody(t, rec)["code"]; got != "engine_unavailable" {
		t.Errorf("code = %v, want engine_unavailable", got)
	}
}

func TestReport_RejectsOtherMethods(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/reports", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// TestReport_IsRateLimited proves the endpoint is published through the shared
// limit wrapper, rather than the wrapper merely existing.
func TestReport_IsRateLimited(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))), func(s *Server) {
		s.limiter = newRateLimiter(1, 1)
	})

	first := postJSON(t, s, "/api/reports", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":10}`)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first request status = %d, want 202: %s", first.Code, first.Body)
	}

	second := postJSON(t, s, "/api/reports", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":10}`)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429: %s", second.Code, second.Body)
	}
	if got := second.Header().Get("Retry-After"); got == "" {
		t.Error("a refused request carried no Retry-After hint")
	}
	if got := decodeBody(t, second)["code"]; got != "rate_limited" {
		t.Errorf("code = %v, want rate_limited", got)
	}
}
