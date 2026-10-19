package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fleetflow/internal/auth"
	authmemory "fleetflow/internal/auth/memory"
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

// testAPIKey is the key every test server accepts. It is a fixed string rather
// than one issued per test, so that a test calling an ingest endpoint does not
// first have to arrange authentication; nothing outside the tests ever holds a
// plaintext key.
const testAPIKey = "flf_test_key_for_the_suite_0123456789"

// testKeyService returns a key service holding exactly testAPIKey. It seeds the
// store directly because Service.Issue generates its own plaintext, and a test
// needs to know the key it is going to present.
func testKeyService(t *testing.T) *auth.Service {
	t.Helper()

	repo := authmemory.New()
	if _, err := repo.Create(context.Background(), auth.Key{
		ID:        "test-key",
		Name:      "test suite",
		Prefix:    auth.DisplayPrefix(testAPIKey),
		Hash:      auth.Hash(testAPIKey),
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seeding the test key: %v", err)
	}
	return auth.NewService(repo)
}

func testServer(t *testing.T, opts ...Option) *Server {
	t.Helper()
	return testServerFor(t, testGraph(t), opts...)
}

// testServerFor builds a server over a specific graph, for the tests that need
// one small enough to contain an unreachable pair.
func testServerFor(t *testing.T, g *graph.Graph, opts ...Option) *Server {
	t.Helper()

	// Tests are noisy enough without a line per request; the logging middleware
	// has its own test that captures into a buffer. A test may override this by
	// passing its own WithLogger, which is applied later.
	all := append([]Option{
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		// Every test server accepts the same key, so a test that calls an ingest
		// endpoint does not first have to arrange authentication. The one test
		// about a server with no key service builds its own server.
		WithAPIKeys(testKeyService(t)),
	}, opts...)
	return NewServer(g, all...)
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

func TestConsoleRoutesDoNotChangeAPIErrorBehavior(t *testing.T) {
	s := testServer(t)

	clientRoute := httptest.NewRecorder()
	s.ServeHTTP(clientRoute, httptest.NewRequest(http.MethodGet, "/operations/trips", nil))
	if clientRoute.Code != http.StatusOK || clientRoute.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("client route answered %d with %q, want the SPA document", clientRoute.Code, clientRoute.Header().Get("Content-Type"))
	}

	wrongMethod := httptest.NewRecorder()
	s.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodGet, "/api/reports", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/reports answered %d, want 405", wrongMethod.Code)
	}

	unknownAPI := httptest.NewRecorder()
	s.ServeHTTP(unknownAPI, httptest.NewRequest(http.MethodGet, "/api/not-a-route", nil))
	if unknownAPI.Code != http.StatusNotFound {
		t.Errorf("GET /api/not-a-route answered %d, want 404", unknownAPI.Code)
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

// decodeBody reads a JSON object response body, failing the test when it is not
// JSON.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
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

	body := decodeBody(t, rec)
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
	if got := decodeBody(t, rec)["database"]; got != "reachable" {
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

	body := decodeBody(t, rec)
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
