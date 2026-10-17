package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fleetflow/internal/graph"
)

// testGraph loads the fixture road graph.
func testGraph(t *testing.T) *graph.Graph {
	t.Helper()

	g, err := graph.LoadFromFile("../../data/test_graph.json")
	if err != nil {
		t.Fatalf("failed to load fixture graph: %v", err)
	}
	return g
}

func testServer(t *testing.T, opts ...Option) *Server {
	t.Helper()
	return NewServer(testGraph(t), opts...)
}

func TestHealthz(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("body = %v, want status ok", body)
	}
}

func TestHealthz_RejectsOtherMethods(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/healthz", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// decodeJSON reads a JSON object body, failing the test when it is not JSON.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	return body
}

func TestReadyz_WithoutDatabase(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := decodeJSON(t, rec)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["graph_loaded"] != true {
		t.Errorf("graph_loaded = %v, want true", body["graph_loaded"])
	}
	if body["database"] != "not_configured" {
		t.Errorf("database = %v, want not_configured", body["database"])
	}
}

func TestReadyz_DatabaseReachable(t *testing.T) {
	s := testServer(t, WithDatabaseCheck(func(context.Context) error { return nil }))
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := decodeJSON(t, rec)["database"]; got != "reachable" {
		t.Errorf("database = %v, want reachable", got)
	}
}

func TestReadyz_DatabaseUnreachable(t *testing.T) {
	s := testServer(t, WithDatabaseCheck(func(context.Context) error {
		return errors.New("connection refused")
	}))
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}

	body := decodeJSON(t, rec)
	if body["status"] != "unavailable" {
		t.Errorf("status = %v, want unavailable", body["status"])
	}
	if body["database"] != "unreachable" {
		t.Errorf("database = %v, want unreachable", body["database"])
	}
}

func TestReadyz_RejectsOtherMethods(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/readyz", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// TestRunStopsOnCancel covers the graceful shutdown contract: cancelling the
// context stops registered components and makes Run return without an error.
func TestRunStopsOnCancel(t *testing.T) {
	stopped := make(chan struct{})
	s := testServer(t, WithStopFunc(func(context.Context) error {
		close(stopped)
		return nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx, "127.0.0.1:0") }()

	cancel()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the registered stop function was not called on shutdown")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() = %v, want nil after cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after the context was cancelled")
	}
}
