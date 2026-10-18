package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// list reads a JSON array field out of a decoded body, failing the test when it is
// missing or is not an array.
func list(t *testing.T, body map[string]any, key string) []any {
	t.Helper()

	value, ok := body[key]
	if !ok {
		t.Fatalf("body has no %q: %v", key, body)
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("%q is %T, want an array: %v", key, value, value)
	}
	return items
}

func TestGraph_ServesEveryNodeAndEdge(t *testing.T) {
	g := testGraph(t)
	s := simServer(t)

	rec := getJSON(t, s, "/api/graph")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	body := decodeBody(t, rec)
	nodes, edges := list(t, body, "nodes"), list(t, body, "edges")
	if len(nodes) != len(g.Nodes) {
		t.Errorf("served %d nodes, want the graph's %d", len(nodes), len(g.Nodes))
	}
	if len(edges) != len(g.Edges) {
		t.Errorf("served %d edges, want the graph's %d", len(edges), len(g.Edges))
	}

	// A client draws from these fields, so the ones it cannot work out for itself
	// have to be there: where an edge runs, and what it costs when empty.
	first, ok := nodes[0].(map[string]any)
	if !ok {
		t.Fatalf("node is %T, want an object", nodes[0])
	}
	for _, key := range []string{"id", "lat", "lon"} {
		if _, ok := first[key]; !ok {
			t.Errorf("a node carries no %q: %v", key, first)
		}
	}
	edge, ok := edges[0].(map[string]any)
	if !ok {
		t.Fatalf("edge is %T, want an object", edges[0])
	}
	for _, key := range []string{"id", "from", "to", "free_flow_s", "capacity", "speed_limit_mps"} {
		if _, ok := edge[key]; !ok {
			t.Errorf("an edge carries no %q: %v", key, edge)
		}
	}
	if free, _ := edge["free_flow_s"].(float64); free <= 0 {
		t.Errorf("edge free_flow_s = %v, want a positive time", free)
	}
}

// The graph is fixed while the server runs, and a client that caches it must get
// the same bytes every time, so nodes cannot come out in map order.
func TestGraph_ServesTheSameOrderEveryTime(t *testing.T) {
	s := simServer(t)

	first := getJSON(t, s, "/api/graph").Body.String()
	for i := 0; i < 5; i++ {
		if next := getJSON(t, s, "/api/graph").Body.String(); next != first {
			t.Fatalf("response %d differs from the first, so the graph looks like it changed", i+2)
		}
	}
}

func TestGraph_IsCacheableAndOpen(t *testing.T) {
	s := simServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 without an API key", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got == "" {
		t.Error("no Cache-Control: a graph that never changes should be cached")
	}
}

func TestGraph_RejectsOtherMethods(t *testing.T) {
	s := simServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/graph", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
