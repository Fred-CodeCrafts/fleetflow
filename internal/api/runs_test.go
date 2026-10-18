package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	runstore "fleetflow/internal/run"
	runmemory "fleetflow/internal/run/memory"
)

// runsServer builds a server that can both run a comparison and keep one, which
// is what the saved-run endpoints need: the record is captured from the run, so
// a server without a run has nothing to save.
func runsServer(t *testing.T) *Server {
	t.Helper()

	s := testServer(t,
		WithEngine(newEngine(t, testGraph(t))),
		WithRuns(runstore.NewService(runmemory.New())),
		// Saving a run needs one to have run, and runs are served only in demo.
		WithDemo(true),
	)
	t.Cleanup(func() { s.stopRun(context.Background()) })
	return s
}

// postNoBody sends a POST with no body at all, the way a client saves the run.
func postNoBody(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
	return rec
}

// TestRuns_SaveThenList is the T39b done-when in a test: a run is stored with its
// seed and metrics, and the list returns it.
func TestRuns_SaveThenList(t *testing.T) {
	s := runsServer(t)

	if rec := postJSON(t, s, "/api/sim/start", `{"seed":7,"vehicles":8,"window_s":0}`); rec.Code != http.StatusAccepted {
		t.Fatalf("start = %d, want 202: %s", rec.Code, rec.Body)
	}

	rec := postNoBody(t, s, "/api/runs")
	if rec.Code != http.StatusCreated {
		t.Fatalf("save = %d, want 201: %s", rec.Code, rec.Body)
	}

	saved := decodeBody(t, rec)
	// The run was started on a server built with WithDemo(true), so the record says
	// so: whether a run owned the weights is part of what it measured.
	if demo, ok := saved["demo"].(bool); !ok || !demo {
		t.Errorf("saved run demo = %v, want true", saved["demo"])
	}
	if id := field(t, saved, "id"); id == "" {
		t.Error("saved run has no id")
	}
	if seed, _ := saved["seed"].(float64); seed != 7 {
		t.Errorf("seed = %v, want the run's 7", saved["seed"])
	}

	// The config document is the run's demand, not an echo of what was asked
	// for: it carries the defaults the request left out, so posting it back
	// starts the same run.
	config, ok := saved["config"].(map[string]any)
	if !ok {
		t.Fatalf("config is not an object: %v", saved["config"])
	}
	if seed, _ := config["seed"].(float64); seed != 7 {
		t.Errorf("config.seed = %v, want 7", config["seed"])
	}
	for _, key := range []string{"vehicles", "fleet_share", "step_s", "window_s", "reroute_every_s", "reroute_threshold"} {
		if _, has := config[key]; !has {
			t.Errorf("config has no %q, so it cannot start the run again", key)
		}
	}

	// The metrics document is the run's own metrics, the object GET /api/metrics
	// reports under "run", with both arms in it.
	metrics, ok := saved["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("metrics is not an object: %v", saved["metrics"])
	}
	for _, key := range []string{"seed", "vehicles", "baseline", "fleet_flow"} {
		if _, has := metrics[key]; !has {
			t.Errorf("metrics has no %q", key)
		}
	}

	list := listOf(t, decodeBody(t, getJSON(t, s, "/api/runs")), "runs")
	if len(list) != 1 {
		t.Fatalf("list returned %d runs, want 1", len(list))
	}
	first, _ := list[0].(map[string]any)
	if got := field(t, first, "id"); got != field(t, saved, "id") {
		t.Errorf("listed id = %q, want the saved %q", got, field(t, saved, "id"))
	}
	if got := field(t, first, "created_at"); got == "" {
		t.Error("listed run has no created_at")
	}
}

// TestRuns_SaveWithoutARunIsRefused keeps the record honest: there is nothing to
// save, and an empty record would read later as a run that produced nothing.
func TestRuns_SaveWithoutARunIsRefused(t *testing.T) {
	s := runsServer(t)

	rec := postNoBody(t, s, "/api/runs")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "no_run" {
		t.Errorf("code = %q, want no_run", code)
	}

	if list := listOf(t, decodeBody(t, getJSON(t, s, "/api/runs")), "runs"); len(list) != 0 {
		t.Errorf("a refused save stored %d runs", len(list))
	}
}

// TestRuns_SaveTakesNoBody pins the contract: the run is captured from the
// server, so a client-supplied body is refused rather than ignored. Ignoring it
// would let a client believe it had chosen what to save.
func TestRuns_SaveTakesNoBody(t *testing.T) {
	s := runsServer(t)
	postJSON(t, s, "/api/sim/start", `{"seed":1,"vehicles":4,"window_s":0}`)

	rec := postJSON(t, s, "/api/runs", `{"seed":99}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "body_not_taken" {
		t.Errorf("code = %q, want body_not_taken", code)
	}
}

// TestRuns_ListRefusesAnUnreadableLimit: a limit the endpoint cannot read is a
// client mistake, not a request for the default page.
func TestRuns_ListRefusesAnUnreadableLimit(t *testing.T) {
	s := runsServer(t)

	rec := getJSON(t, s, "/api/runs?limit=lots")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "limit_unreadable" {
		t.Errorf("code = %q, want limit_unreadable", code)
	}
}

// TestRuns_ListClampsThePageSize checks the boundary the service owns: a limit
// past the cap is trimmed rather than refused, because asking for too much is a
// reasonable request.
func TestRuns_ListClampsThePageSize(t *testing.T) {
	s := runsServer(t)
	postJSON(t, s, "/api/sim/start", `{"seed":1,"vehicles":4,"window_s":0}`)
	for i := 0; i < 2; i++ {
		postNoBody(t, s, "/api/runs")
	}

	rec := getJSON(t, s, "/api/runs?limit=100000")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if list := listOf(t, decodeBody(t, rec), "runs"); len(list) != 2 {
		t.Errorf("list returned %d runs, want both", len(list))
	}
}

// TestRuns_SavingTwiceKeepsBoth: saving is not an update. A run saved again is a
// second record, because the second save may well be a later moment of the run.
func TestRuns_SavingTwiceKeepsBoth(t *testing.T) {
	s := runsServer(t)
	postJSON(t, s, "/api/sim/start", `{"seed":5,"vehicles":4,"window_s":0}`)

	first := field(t, decodeBody(t, postNoBody(t, s, "/api/runs")), "id")
	second := field(t, decodeBody(t, postNoBody(t, s, "/api/runs")), "id")
	if first == second {
		t.Fatalf("both saves returned the id %q", first)
	}

	if list := listOf(t, decodeBody(t, getJSON(t, s, "/api/runs")), "runs"); len(list) != 2 {
		t.Errorf("list returned %d runs, want 2", len(list))
	}
}

// TestRuns_WithoutTheServiceAnswer503 keeps the server from dereferencing a nil
// service: a server built without WithRuns has the endpoints but cannot serve.
func TestRuns_WithoutTheServiceAnswer503(t *testing.T) {
	s := testServer(t)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/runs"},
		{http.MethodPost, "/api/runs"},
	} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader("")))

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s = %d, want 503", tc.method, tc.path, rec.Code)
		}
		if code := field(t, decodeBody(t, rec), "code"); code != "runs_unavailable" {
			t.Errorf("%s %s code = %q, want runs_unavailable", tc.method, tc.path, code)
		}
	}
}
