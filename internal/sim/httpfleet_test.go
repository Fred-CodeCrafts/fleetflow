package sim_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"fleetflow/internal/api"
	"fleetflow/internal/auth"
	authmemory "fleetflow/internal/auth/memory"
	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
	"fleetflow/internal/sim"
	"fleetflow/internal/weights"
)

// clientKey is the key the test server accepts. The service issues a key once and
// keeps only its digest, which is not something a test can arrange for a key it
// already knows, so the store is seeded directly.
const clientKey = "flf_sim_client_test_0123456789012345"

func loadFixture(t *testing.T) *graph.Graph {
	t.Helper()

	g, err := graph.LoadFromFile("../../data/test_graph.json")
	if err != nil {
		t.Fatalf("failed to load fixture graph: %v", err)
	}
	return g
}

// congestibleFixture loads the fixture graph with its capacities lowered. The
// fixture is four nodes across a few hundred metres, so its real capacities are
// far above the traffic these tests put on it and congestion would round away to
// nothing. Lowering them makes the effect of a report observable, which is what
// the run assertions read.
func congestibleFixture(t *testing.T) *graph.Graph {
	t.Helper()

	g := loadFixture(t)
	for i := range g.Edges {
		g.Edges[i].Capacity = 4
	}
	return g
}

func edgeIndex(t *testing.T, g *graph.Graph, id string) int {
	t.Helper()

	_, idx, ok := g.EdgeByID(id)
	if !ok {
		t.Fatalf("edge %s not found", id)
	}
	return idx
}

// traffic counts the requests the API accepted, so a test can prove a simulation
// really went over the wire rather than through a shortcut.
type traffic struct {
	next http.Handler

	mu       sync.Mutex
	accepted map[string]int
}

func newTraffic(next http.Handler) *traffic {
	return &traffic{next: next, accepted: make(map[string]int)}
}

// ServeHTTP implements http.Handler.
func (h *traffic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	h.next.ServeHTTP(rec, r)

	if rec.status >= 200 && rec.status < 300 {
		h.mu.Lock()
		h.accepted[r.URL.Path]++
		h.mu.Unlock()
	}
}

// acceptedOn reports how many requests to path the API accepted.
func (h *traffic) acceptedOn(path string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.accepted[path]
}

// statusRecorder remembers the status a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// newTestAPI starts a real API server over g, backed by a fresh engine, and
// returns it with the engine so a test can drive customization on the simulation
// clock the way the server's own run loop will.
func newTestAPI(t *testing.T, g *graph.Graph) (*httptest.Server, *engine.Engine, *traffic) {
	t.Helper()

	eng, err := engine.New(g, engine.NewDijkstraRouter(g), weights.DefaultParams())
	if err != nil {
		t.Fatalf("failed to build engine: %v", err)
	}

	srv := api.NewServer(g,
		api.WithEngine(eng),
		api.WithAPIKeys(keyService(t)),
		// The simulation is a client of these endpoints and sends far more
		// requests than any operator, so the limit is taken out of its way; the
		// limit itself has its own tests.
		api.WithRateLimit(1e6, 1e6),
		api.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)

	traffic := newTraffic(srv)
	ts := httptest.NewServer(traffic)
	t.Cleanup(ts.Close)
	return ts, eng, traffic
}

func keyService(t *testing.T) *auth.Service {
	t.Helper()

	repo := authmemory.New()
	if _, err := repo.Create(context.Background(), auth.Key{
		ID:        "sim-client-test",
		Name:      "sim client test",
		Prefix:    auth.DisplayPrefix(clientKey),
		Hash:      auth.Hash(clientKey),
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seeding the test key: %v", err)
	}
	return auth.NewService(repo)
}

// runThroughAPI runs a seeded simulation whose fleet vehicles speak to the API,
// and returns the collected metrics. Weights are refreshed on the simulation
// clock the way the server's own run loop will, because an HTTP client cannot
// reach the engine to refresh them itself.
func runThroughAPI(t *testing.T, g *graph.Graph, fleet sim.FleetService, eng *engine.Engine, cfg sim.Config, steps int) sim.Metrics {
	t.Helper()

	s, err := sim.New(g, cfg)
	if err != nil {
		t.Fatalf("failed to build simulation: %v", err)
	}
	s.SetFleetService(fleet)

	every := int(eng.CustomizeEvery().Seconds() / cfg.StepS)
	if every < 1 {
		every = 1
	}
	for i := 1; i <= steps; i++ {
		s.Step()
		if i%every == 0 {
			eng.DecayStale(s.ReportTime())
			eng.Customize()
		}
	}
	return s.Collect()
}

// demoConfig is a run big enough to congest the fixture graph but quick over a
// loopback HTTP server.
func demoConfig() sim.Config {
	cfg := sim.DefaultConfig()
	cfg.Seed = 7
	cfg.Vehicles = 20
	cfg.FleetShare = 1
	cfg.StepS = 5
	cfg.WindowS = 600
	cfg.RerouteEveryS = 30
	return cfg
}

// TestHTTPFleet_RunsADemoRunThroughThePublicAPI is T40a's done-when: the
// simulator's fleet service is an HTTP client against /api/reports and
// /api/route, so a run goes over the same validated path an external client uses.
func TestHTTPFleet_RunsADemoRunThroughThePublicAPI(t *testing.T) {
	g := congestibleFixture(t)
	ts, eng, traffic := newTestAPI(t, g)
	fleet := sim.NewHTTPFleet(ts.URL, clientKey, g, ts.Client())

	metrics := runThroughAPI(t, g, fleet, eng, demoConfig(), 240)

	if metrics.CompletedTrips == 0 {
		t.Fatal("no trip completed through the public API")
	}
	if metrics.UnroutableVehicles != 0 {
		t.Errorf("UnroutableVehicles = %d, want 0: a route request over the API failed",
			metrics.UnroutableVehicles)
	}

	// The traffic really went over the wire, and the API accepted both kinds.
	// Reports may arrive singly or batched: congestion is per edge, so the
	// simulator sends one observation per edge per step and the client batches
	// them into /api/reports/batch. Either route proves the reports went over the
	// public, authenticated API rather than around it.
	if got := traffic.acceptedOn("/api/reports") + traffic.acceptedOn("/api/reports/batch"); got == 0 {
		t.Error("the API accepted no reports: simulated traffic did not use /api/reports")
	}
	if got := traffic.acceptedOn("/api/route"); got == 0 {
		t.Error("the API accepted no route queries: simulated traffic did not use /api/route")
	}

	// A report must have reached the weights: the only way an edge grows past its
	// free-flow cost is for a report to have been folded in and published.
	m := eng.ActiveMetric()
	congested := false
	for i, e := range g.Edges {
		if m.Cost(i) > e.FreeFlowS {
			congested = true
			break
		}
	}
	if !congested {
		t.Error("no edge grew past free flow: no report reached the engine")
	}
}

// TestHTTPFleet_SameSeedGivesTheSameRun covers the deterministic half of T40a's
// done-when: reports are sent synchronously, so the same seed produces the same
// run even when every call goes over HTTP.
func TestHTTPFleet_SameSeedGivesTheSameRun(t *testing.T) {
	g := congestibleFixture(t)
	cfg := demoConfig()

	run := func() sim.Metrics {
		// A fresh server and engine per run, so the second run does not start
		// from the first run's congestion.
		ts, eng, _ := newTestAPI(t, g)
		fleet := sim.NewHTTPFleet(ts.URL, clientKey, g, ts.Client())
		return runThroughAPI(t, g, fleet, eng, cfg, 240)
	}

	first := run()
	second := run()

	if !reflect.DeepEqual(first, second) {
		t.Errorf("two runs on seed %d differ:\nfirst  = %+v\nsecond = %+v", cfg.Seed, first, second)
	}
	if first.CompletedTrips == 0 {
		t.Fatal("the run completed no trips, so the comparison proves nothing")
	}
}

// TestHTTPFleet_PricesAHeldPathOverTheWire checks the reroute half of the client:
// the plan and the price of the path a vehicle already holds come back from one
// call, on one snapshot.
func TestHTTPFleet_PricesAHeldPathOverTheWire(t *testing.T) {
	g := loadFixture(t)
	ts, eng, _ := newTestAPI(t, g)
	fleet := sim.NewHTTPFleet(ts.URL, clientKey, g, ts.Client())

	held := []int{edgeIndex(t, g, "e_AB"), edgeIndex(t, g, "e_BD")}
	answer, err := fleet.Route("A", "D", held)
	if err != nil {
		t.Fatalf("Route() = %v", err)
	}

	if !answer.HeldPriced {
		t.Fatal("HeldPriced = false for a query that sent a held path")
	}
	if want := eng.ActiveMetric().Version(); answer.Version != want {
		t.Errorf("version = %d, want the active snapshot %d", answer.Version, want)
	}
	if answer.HeldCostS <= answer.Path.CostS {
		t.Errorf("held cost = %v, want more than the cheaper plan %v (A->B->D is longer than A->C->D)",
			answer.HeldCostS, answer.Path.CostS)
	}

	// The plan comes back as IDs and must be mapped onto the graph.
	if len(answer.Path.Edges) == 0 {
		t.Fatal("the plan is empty for a reachable pair")
	}
	for _, idx := range answer.Path.Edges {
		if idx < 0 || idx >= len(g.Edges) {
			t.Fatalf("plan names edge index %d, outside the graph", idx)
		}
	}

	// A query with no held path must not be priced.
	plain, err := fleet.Route("A", "D", nil)
	if err != nil {
		t.Fatalf("Route() = %v", err)
	}
	if plain.HeldPriced {
		t.Error("HeldPriced = true for a query that sent no held path")
	}
}

// TestHTTPFleet_GoesThroughAPIPolicy checks that simulated traffic really is on
// the public path: the API's authentication and validation decide, not the
// simulator.
func TestHTTPFleet_GoesThroughAPIPolicy(t *testing.T) {
	g := loadFixture(t)
	ts, _, _ := newTestAPI(t, g)

	wrongKey := sim.NewHTTPFleet(ts.URL, "flf_not_a_real_key", g, ts.Client())
	if _, err := wrongKey.Route("A", "D", nil); err == nil {
		t.Error("a route query with a wrong key succeeded")
	}

	fleet := sim.NewHTTPFleet(ts.URL, clientKey, g, ts.Client())
	if err := fleet.Report("v1", "e_AB", 0, time.Unix(0, 0)); err == nil {
		t.Error("a report with an out-of-range speed was accepted")
	}
	if err := fleet.Report("v1", "nowhere", 10, time.Unix(0, 0)); err == nil {
		t.Error("a report for an edge not in the graph was accepted")
	}
}
