package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fleetflow/internal/graph"
)

// oneWayGraph is a two-node graph with a single A->B edge, so B->A has a
// well-formed query with no route to answer it. The fixture graph is fully
// connected and cannot express that case.
func oneWayGraph(t *testing.T) *graph.Graph {
	t.Helper()

	g := graph.New()
	for _, id := range []string{"A", "B"} {
		if err := g.AddNode(graph.Node{ID: id}); err != nil {
			t.Fatalf("failed to add node %s: %v", id, err)
		}
	}
	edge := graph.Edge{ID: "e_AB", From: "A", To: "B", LengthM: 100, SpeedLimitMPS: 10, Lanes: 1}
	if err := g.AddEdge(edge); err != nil {
		t.Fatalf("failed to add edge: %v", err)
	}
	return g
}

func TestRoute_ReturnsEdgesCostAndVersion(t *testing.T) {
	g := testGraph(t)
	e := newEngine(t, g)
	s := testServer(t, WithEngine(e))

	rec := postJSON(t, s, "/api/route", `{"from_node":"A","to_node":"D"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	var body struct {
		Edges         []string `json:"edges"`
		CostS         float64  `json:"cost_s"`
		MetricVersion uint64   `json:"metric_version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the route shape: %v", err)
	}

	if len(body.Edges) == 0 {
		t.Fatal("edges is empty for a reachable pair")
	}
	// On free-flow weights A->C->D is cheaper than A->B->D, so the ordered edges
	// are what a client can follow.
	for i, id := range body.Edges {
		if _, _, ok := g.EdgeByID(id); !ok {
			t.Errorf("edges[%d] = %q, which is not an edge of the graph", i, id)
		}
	}
	if body.CostS <= 0 {
		t.Errorf("cost_s = %v, want a positive cost", body.CostS)
	}
	if want := e.ActiveMetric().Version(); body.MetricVersion != want {
		t.Errorf("metric_version = %d, want the active snapshot %d", body.MetricVersion, want)
	}
}

func TestRoute_FollowsTheCurrentWeights(t *testing.T) {
	g := testGraph(t)
	e := newEngine(t, g)
	s := testServer(t, WithEngine(e))

	first := postJSON(t, s, "/api/route", `{"from_node":"A","to_node":"D"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", first.Code, first.Body)
	}

	// Congest the cheaper leg so the engine has something new to publish.
	if rec := postJSON(t, s, "/api/reports",
		`{"vehicle_id":"v1","edge_id":"e_AC","speed_mps":1}`); rec.Code != http.StatusAccepted {
		t.Fatalf("report status = %d, want 202: %s", rec.Code, rec.Body)
	}
	e.Customize()

	second := postJSON(t, s, "/api/route", `{"from_node":"A","to_node":"D"}`)
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", second.Code, second.Body)
	}

	var before, after struct {
		Edges         []string `json:"edges"`
		CostS         float64  `json:"cost_s"`
		MetricVersion uint64   `json:"metric_version"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &before); err != nil {
		t.Fatalf("first response is not the route shape: %v", err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &after); err != nil {
		t.Fatalf("second response is not the route shape: %v", err)
	}

	if after.MetricVersion <= before.MetricVersion {
		t.Errorf("metric_version = %d after a customization, want more than %d",
			after.MetricVersion, before.MetricVersion)
	}
	if after.CostS <= before.CostS {
		t.Errorf("cost_s = %v after congesting the chosen leg, want more than %v",
			after.CostS, before.CostS)
	}
}

func TestRoute_NodeToItselfIsAnEmptyList(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	rec := postJSON(t, s, "/api/route", `{"from_node":"A","to_node":"A"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	// An empty list, not null: a client that iterates the field should not have
	// to check for nil.
	if edges, ok := decodeBody(t, rec)["edges"]; !ok || edges == nil {
		t.Errorf("edges = %v, want an empty JSON array", edges)
	}
}

func TestRoute_RejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{"missing from", `{"to_node":"D"}`, codeFromNodeRequired},
		{"missing to", `{"from_node":"A"}`, codeToNodeRequired},
	}

	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postJSON(t, s, "/api/route", tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
			if got := decodeBody(t, rec)["code"]; got != tt.code {
				t.Errorf("code = %v, want %s", got, tt.code)
			}
		})
	}
}

func TestRoute_UnknownNodeNamesTheNode(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	rec := postJSON(t, s, "/api/route", `{"from_node":"nowhere","to_node":"D"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}

	body := decodeBody(t, rec)
	if body["code"] != "unknown_node" {
		t.Errorf("code = %v, want unknown_node", body["code"])
	}
	if body["message"] == "internal server error" {
		t.Error("the client was handed a generic internal failure")
	}
}

// TestRoute_UnreachableDestinationIsAClearError is the T38 done-when: a
// destination that cannot be reached must come back as a clear error, not an
// empty path or a 200.
func TestRoute_UnreachableDestinationIsAClearError(t *testing.T) {
	g := oneWayGraph(t)
	s := testServerFor(t, g, WithEngine(newEngine(t, g)))

	rec := postJSON(t, s, "/api/route", `{"from_node":"B","to_node":"A"}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}

	body := decodeBody(t, rec)
	if body["code"] != "no_path" {
		t.Errorf("code = %v, want no_path", body["code"])
	}
	if body["message"] == "internal server error" {
		t.Error("the client was handed a generic internal failure")
	}
}

func TestRoute_WithoutAnEngineIsUnavailable(t *testing.T) {
	s := testServer(t)

	rec := postJSON(t, s, "/api/route", `{"from_node":"A","to_node":"D"}`)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if got := decodeBody(t, rec)["code"]; got != "engine_unavailable" {
		t.Errorf("code = %v, want engine_unavailable", got)
	}
}

func TestRoute_RejectsOtherMethods(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/route", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
