package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
	"fleetflow/internal/sim"
)

// simServer builds a server that can run a simulation, and stops any run it left
// going when the test ends, so no clock keeps ticking into the next test.
func simServer(t *testing.T) *Server {
	t.Helper()

	s := testServer(t, WithEngine(newEngine(t, testGraph(t))), WithDemo(true))
	t.Cleanup(func() { s.stopRun(context.Background()) })
	return s
}

// simServerWithEngine builds a simulation server over a specific engine, for the
// tests that inspect the weights a run starts from.
func simServerWithEngine(t *testing.T, e *engine.Engine) *Server {
	t.Helper()

	s := testServer(t, WithEngine(e), WithDemo(true))
	t.Cleanup(func() { s.stopRun(context.Background()) })
	return s
}

// baselineEngine builds the static engine a run's baseline arm routes on.
func baselineEngine(t *testing.T, g *graph.Graph) *engine.Engine {
	t.Helper()

	e, err := engine.NewBaseline(g, engine.NewDijkstraRouter(g))
	if err != nil {
		t.Fatalf("failed to build the baseline engine: %v", err)
	}
	return e
}

// quickRunConfig is a run small enough that both arms settle in a few hundred
// steps.
func quickRunConfig() sim.Config {
	cfg := sim.DefaultConfig()
	cfg.Seed = 3
	cfg.Vehicles = 8
	cfg.WindowS = 0
	cfg.StepS = 1
	return cfg
}

func TestSimStart_StartsTheRunTheRequestAsksFor(t *testing.T) {
	s := simServer(t)

	rec := postJSON(t, s, "/api/sim/start",
		`{"seed":42,"vehicles":5,"fleet_share":0.5,"step_s":2,"window_s":60}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body)
	}

	body := decodeBody(t, rec)
	if seed, _ := body["seed"].(float64); seed != 42 {
		t.Errorf("seed = %v, want 42", body["seed"])
	}
	if vehicles, _ := body["vehicles"].(float64); vehicles != 5 {
		t.Errorf("vehicles = %v, want 5", body["vehicles"])
	}
	if share, _ := body["fleet_share"].(float64); share != 0.5 {
		t.Errorf("fleet_share = %v, want 0.5", body["fleet_share"])
	}
	if running, _ := body["running"].(bool); !running {
		t.Error("running = false for a run that was just started")
	}

	s.runMu.Lock()
	live := s.run
	s.runMu.Unlock()
	if live == nil {
		t.Fatal("start reported a run but the server has none")
	}
	if live.cfg.Seed != 42 || live.cfg.Vehicles != 5 {
		t.Errorf("run config = seed %d, vehicles %d, want the requested ones",
			live.cfg.Seed, live.cfg.Vehicles)
	}
}

// The dashboard owns these controls, like the record endpoints, so they are not
// behind the API key that the ingest endpoints require.
func TestSimControl_IsOpenToTheDashboard(t *testing.T) {
	s := simServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/sim/reset", nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 without an API key", rec.Code)
	}
}

// TestSimStart_IsAbsentWithoutDemo is the gate: a deployment that did not ask for a
// simulation cannot start one, and gets the same 404 as any other unregistered path
// rather than a refusal from a handler that exists.
func TestSimStart_IsAbsentWithoutDemo(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	for _, path := range []string{"/api/sim/start", "/api/sim/reset"} {
		rec := postJSON(t, s, path, `{"vehicles":2,"window_s":10}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s status = %d without -demo, want 404: %s", path, rec.Code, rec.Body)
		}
	}
}

// TestSimStart_IsServedWithDemo is the other side of the gate, so the 404 above
// cannot pass by the routes being broken for everybody.
func TestSimStart_IsServedWithDemo(t *testing.T) {
	s := simServer(t)

	if rec := postJSON(t, s, "/api/sim/start", `{"vehicles":2,"window_s":10}`); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d with -demo, want 202: %s", rec.Code, rec.Body)
	}
}

// TestSimStart_RefusesASecondStart is the correctness rule: a live comparison is
// not replaced. Both the running run and its clock are left alone, so a caller
// cannot discard somebody else's comparison by starting another one.
func TestSimStart_RefusesASecondStart(t *testing.T) {
	s := simServer(t)

	first := postJSON(t, s, "/api/sim/start", `{"seed":1,"vehicles":20,"window_s":600}`)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first start status = %d, want 202: %s", first.Code, first.Body)
	}

	rec := postJSON(t, s, "/api/sim/start", `{"seed":2,"vehicles":3,"window_s":60}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second start status = %d, want 409: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "run_in_progress" {
		t.Errorf("code = %q, want run_in_progress", code)
	}

	s.runMu.Lock()
	live := s.run
	s.runMu.Unlock()

	if live == nil {
		t.Fatal("the second start stopped the first run")
	}
	if live.cfg.Seed != 1 || live.cfg.Vehicles != 20 {
		t.Errorf("current run = seed %d, vehicles %d, want the first start untouched",
			live.cfg.Seed, live.cfg.Vehicles)
	}
}

// TestSimStart_ClaimsTheSlotBeforeBuilding keeps two starts that arrive together
// from both finding the slot empty: the claim is taken before any of the work, so
// one of them is refused.
func TestSimStart_ClaimsTheSlotBeforeBuilding(t *testing.T) {
	s := simServer(t)

	if !s.claimRun() {
		t.Fatal("claimRun refused an empty slot")
	}
	if s.claimRun() {
		t.Error("claimRun took a slot that was already claimed")
	}

	s.releaseRun()
	if !s.claimRun() {
		t.Error("claimRun refused the slot after it was released")
	}
}

// TestSimStart_ResetsTheWeights is the other half of owning the weights: a run
// starts from free flow, so its numbers do not depend on what ran before it.
func TestSimStart_ResetsTheWeights(t *testing.T) {
	g := testGraph(t)
	e := newEngine(t, g)
	s := simServerWithEngine(t, e)

	_, index, ok := g.EdgeByID("e_AB")
	if !ok {
		t.Fatal("the fixture graph has no e_AB edge")
	}
	freeFlow := e.ActiveMetric().Cost(index)

	// Congest the edge the way a previous run would have, and confirm it moved.
	if err := e.Report("e_AB", 1, time.Now()); err != nil {
		t.Fatalf("report: %v", err)
	}
	e.Customize()
	if congested := e.ActiveMetric().Cost(index); congested <= freeFlow {
		t.Fatalf("cost after a crawl = %v, want more than free flow %v", congested, freeFlow)
	}

	if rec := postJSON(t, s, "/api/sim/start", `{"vehicles":2,"window_s":10}`); rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202: %s", rec.Code, rec.Body)
	}

	if got := e.ActiveMetric().Cost(index); got != freeFlow {
		t.Errorf("cost after a start = %v, want the run to begin at free flow %v", got, freeFlow)
	}
}

func TestSimReset_StopsAndClearsTheRun(t *testing.T) {
	s := simServer(t)
	postJSON(t, s, "/api/sim/start", `{"vehicles":20,"window_s":600}`)

	rec := postJSON(t, s, "/api/sim/reset", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body)
	}

	s.runMu.Lock()
	live := s.run
	s.runMu.Unlock()
	if live != nil {
		t.Error("reset left a run behind")
	}
}

func TestSimReset_WithNothingRunningIsStillNoContent(t *testing.T) {
	s := simServer(t)

	rec := postJSON(t, s, "/api/sim/reset", "")

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}

func TestSimStart_RejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{"negative vehicles", `{"vehicles":-1}`, codeVehiclesOutOfRange},
		{"too many vehicles", `{"vehicles":99999}`, codeVehiclesOutOfRange},
		{"fleet share above one", `{"fleet_share":1.5}`, codeFleetShareOutOfRange},
		{"negative fleet share", `{"fleet_share":-0.1}`, codeFleetShareOutOfRange},
		{"zero step", `{"step_s":0}`, codeStepOutOfRange},
		{"negative window", `{"window_s":-1}`, codeWindowOutOfRange},
		{"negative reroute interval", `{"reroute_every_s":-1}`, codeRerouteEveryOutOfRange},
		{"threshold of one", `{"reroute_threshold":1}`, codeRerouteThresholdOutOfRan},
	}

	s := simServer(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postJSON(t, s, "/api/sim/start", tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
			if got := decodeBody(t, rec)["code"]; got != tt.code {
				t.Errorf("code = %v, want %s", got, tt.code)
			}
		})
	}
}

func TestSimStart_WithoutAnEngineIsUnavailable(t *testing.T) {
	// Demo is on, so the route exists: the 503 is about the engine, not the gate.
	s := testServer(t, WithDemo(true))

	rec := postJSON(t, s, "/api/sim/start", `{}`)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if got := decodeBody(t, rec)["code"]; got != "engine_unavailable" {
		t.Errorf("code = %v, want engine_unavailable", got)
	}
}

func TestSimStart_RejectsOtherMethods(t *testing.T) {
	s := simServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sim/start", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// TestRun_StepsBothArmsFromOneDemand covers the comparison itself without the
// wall clock: one run drives the same seeded demand through both engines and
// publishes both results.
func TestRun_StepsBothArmsFromOneDemand(t *testing.T) {
	g := testGraph(t)
	cfg := quickRunConfig()

	live, err := newRun(g, cfg, baselineEngine(t, g), sim.NewEngineFleet(newEngine(t, g)),
		func() bool { return true }, make(chan struct{}))
	if err != nil {
		t.Fatalf("newRun() = %v", err)
	}

	for i := 0; i < 400; i++ {
		live.advance()
	}

	snapshot := live.snapshot()
	if snapshot.Metrics.Baseline.CompletedTrips == 0 {
		t.Error("the baseline arm completed no trips")
	}
	if snapshot.Metrics.FleetFlow.CompletedTrips == 0 {
		t.Error("the fleetflow arm completed no trips")
	}
	if snapshot.Metrics.Baseline.TotalVehicles != snapshot.Metrics.FleetFlow.TotalVehicles {
		t.Errorf("the arms ran different demand: %d vs %d vehicles",
			snapshot.Metrics.Baseline.TotalVehicles, snapshot.Metrics.FleetFlow.TotalVehicles)
	}
	if !live.settled() {
		t.Error("both arms completed trips but the run did not settle")
	}
}

// plainFleet is a FleetService that is deliberately not a sim.FleetCustomizer,
// like the HTTP client: it can route and report but cannot reach the engine to
// refresh weights.
type plainFleet struct{ sim.FleetService }

// TestSimStart_RefreshesWeightsForAnOutOfProcessFleetService covers the one thing
// a fleet service over the wire cannot do for itself. Without the refresh a run
// whose traffic goes through the API would route on a single snapshot from start
// to finish. The stream is the reason the run moves at all (T44), so a viewer
// holds one open.
func TestSimStart_RefreshesWeightsForAnOutOfProcessFleetService(t *testing.T) {
	g := testGraph(t)
	eng := newEngine(t, g)

	s := testServerFor(t, g,
		WithEngine(eng),
		WithDemo(true),
		WithFleetService(func(e *engine.Engine) sim.FleetService {
			return plainFleet{FleetService: sim.NewEngineFleet(e)}
		}),
	)
	t.Cleanup(func() { s.stopRun(context.Background()) })

	before := eng.ActiveMetric().Version()
	postJSON(t, s, "/api/sim/start", `{"vehicles":8,"fleet_share":1,"window_s":0,"step_s":1}`)
	openStream(t, s)

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if eng.ActiveMetric().Version() > before {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Errorf("metric version stayed at %d: the run's clock never refreshed the weights", before)
}

// TestSimStart_AdvancesWhileWatched checks the goroutine the endpoints rely on: a
// run started over HTTP makes progress without anyone stepping it, as long as a
// stream client is holding it open (T44).
func TestSimStart_AdvancesWhileWatched(t *testing.T) {
	s := simServer(t)
	postJSON(t, s, "/api/sim/start", `{"seed":3,"vehicles":4,"window_s":0,"step_s":1}`)
	openStream(t, s)

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		s.runMu.Lock()
		live := s.run
		s.runMu.Unlock()
		if live == nil {
			t.Fatal("the run vanished while it was going")
		}
		if live.snapshot().Metrics.FleetFlow.CompletedTrips > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatal("the run's clock completed no trips within the deadline")
}

// TestSimStart_DoesNotMoveWithNobodyWatching is the other half of T44: with the
// browser tab closed there is no viewer, and Cloud Run throttles CPU outside a
// request, so a run that kept stepping would be invisible work at somebody else's
// expense.
func TestSimStart_DoesNotMoveWithNobodyWatching(t *testing.T) {
	s := simServer(t)
	postJSON(t, s, "/api/sim/start", `{"seed":3,"vehicles":4,"window_s":0,"step_s":1}`)

	// Many ticks' worth of wall time, and the run must not have spent one step of
	// it.
	time.Sleep(20 * tickInterval)

	live := s.currentRun()
	if live == nil {
		t.Fatal("the run vanished before it was watched")
	}
	if now := live.snapshot().NowS; now != 0 {
		t.Errorf("NowS = %v after nobody watched it, want the clock untouched at 0", now)
	}
}
