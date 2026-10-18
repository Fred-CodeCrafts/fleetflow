package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"fleetflow/internal/engine"
	"fleetflow/internal/sim"
)

// countingFleet is a FleetService that answers or fails on demand and keeps no
// state, so a test can drive the wrapper without an engine or a graph.
type countingFleet struct {
	routeErr  error
	reportErr error
}

func (f countingFleet) Route(fromID, toID string, held []int) (engine.RouteAnswer, error) {
	if f.routeErr != nil {
		return engine.RouteAnswer{}, f.routeErr
	}
	return engine.RouteAnswer{Path: &engine.Path{Edges: []int{0}}}, nil
}

func (f countingFleet) Report(vehicleID, edgeID string, speedMPS float64, now time.Time) error {
	return f.reportErr
}

// customCountingFleet is the same, but also a FleetCustomizer, so the wrapper's
// type-assertion behaviour can be checked against it.
type customCountingFleet struct{ countingFleet }

func (customCountingFleet) CustomizeEvery() time.Duration { return 7 * time.Second }
func (customCountingFleet) DecayStale(time.Time)          {}
func (customCountingFleet) Customize() *engine.Metric     { return nil }

// logBuffer returns a logger writing JSON lines into buf.
func logBuffer(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, nil))
}

// lines returns the log lines written so far, parsed.
func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var out []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("log line is not JSON: %v (%s)", err, raw)
		}
		out = append(out, got)
	}
	return out
}

// TestObservedFleet_CountsTheTraffic is the property the finished-run line rests on:
// the wrapper knows how many calls crossed the wire, so a run can state it.
func TestObservedFleet_CountsTheTraffic(t *testing.T) {
	var buf bytes.Buffer
	wrapped := observedFleetOf(countingFleet{}, logBuffer(&buf))

	for i := 0; i < 3; i++ {
		if _, err := wrapped.Route("A", "B", nil); err != nil {
			t.Fatalf("Route: %v", err)
		}
		if err := wrapped.Report("v1", "e_AB", 10, time.Now()); err != nil {
			t.Fatalf("Report: %v", err)
		}
	}

	counted, ok := wrapped.(*observedFleet)
	if !ok {
		t.Fatalf("observedFleetOf returned %T", wrapped)
	}

	got := counted.traffic()
	if got.Routes != 3 || got.Reports != 3 {
		t.Errorf("traffic = %+v, want 3 routes and 3 reports", got)
	}
	if got.RouteFailures != 0 || got.ReportFailures != 0 {
		t.Errorf("traffic = %+v, want no failures", got)
	}
	if buf.Len() != 0 {
		t.Errorf("a healthy run logged something: %s", buf.String())
	}
}

// TestObservedFleet_LogsTheFirstFailureOfEachKind keeps a broken run diagnosable
// without drowning the cause: the first route failure and the first report failure
// are written with detail, the rest are only counted.
func TestObservedFleet_LogsTheFirstFailureOfEachKind(t *testing.T) {
	var buf bytes.Buffer
	wrapped := observedFleetOf(countingFleet{
		routeErr:  errors.New("401 unauthenticated"),
		reportErr: errors.New("422 speed out of range"),
	}, logBuffer(&buf))

	for i := 0; i < 50; i++ {
		_, _ = wrapped.Route("A", "B", nil)
		_ = wrapped.Report("v1", "e_AB", 10, time.Now())
	}

	got := lines(t, &buf)
	if len(got) != 2 {
		t.Fatalf("logged %d lines for 100 failures, want 2 (one per kind): %v", len(got), got)
	}

	if got[0]["msg"] != "a simulated route query failed" {
		t.Errorf("first line = %v, want the route failure", got[0]["msg"])
	}
	if msg, _ := got[0]["error"].(string); !strings.Contains(msg, "401") {
		t.Errorf("route failure lost its cause: %v", got[0]["error"])
	}
	if got[1]["msg"] != "a simulated report failed" {
		t.Errorf("second line = %v, want the report failure", got[1]["msg"])
	}
	if edge, _ := got[1]["edge"].(string); edge != "e_AB" {
		t.Errorf("report failure lost the edge: %v", got[1]["edge"])
	}

	counted := wrapped.(*observedFleet)
	if tr := counted.traffic(); tr.RouteFailures != 50 || tr.ReportFailures != 50 {
		t.Errorf("traffic = %+v, want every failure counted", tr)
	}
}

// TestObservedFleet_KeepsTheCustomizer is the subtle one. The clock finds its weight
// refresh by asserting the service implements sim.FleetCustomizer, so a wrapper that
// always implemented it would turn a non-customizer into one with a zero interval,
// and a wrapper that never implemented it would silently stop the refresh.
func TestObservedFleet_KeepsTheCustomizer(t *testing.T) {
	plain := observedFleetOf(countingFleet{}, slog.Default())
	if _, ok := plain.(sim.FleetCustomizer); ok {
		t.Error("wrapping made a non-customizer look like a customizer")
	}

	custom := observedFleetOf(customCountingFleet{}, slog.Default())
	customizer, ok := custom.(sim.FleetCustomizer)
	if !ok {
		t.Fatal("wrapping lost the customizer")
	}
	if got := customizer.CustomizeEvery(); got != 7*time.Second {
		t.Errorf("CustomizeEvery() = %v, want the wrapped 7s", got)
	}
}

// TestObservedFleet_WithoutALoggerWrapsNothing keeps the decorator out of the way
// when there is nothing to report to: the service is handed back as it came, so a
// test or an in-process arm pays nothing for it.
func TestObservedFleet_WithoutALoggerWrapsNothing(t *testing.T) {
	inner := countingFleet{}

	if got := observedFleetOf(inner, nil); got != inner {
		t.Errorf("observedFleetOf(_, nil) = %T, want the original service", got)
	}
}

// TestRunReport_WarnsWhenNothingCrossedTheWire is the line that tells a good run
// from a silently empty one. Both arms settle and report trip times either way; only
// the traffic counts distinguish them, so a run whose every call failed must say so
// at a level somebody will notice.
func TestRunReport_WarnsWhenNothingCrossedTheWire(t *testing.T) {
	g := testGraph(t)
	cfg := quickRunConfig()

	var buf bytes.Buffer
	logger := logBuffer(&buf)

	// Every call fails, which is what a broken wire looks like from inside the run:
	// the vehicles still move, but on weights nothing ever updated.
	broken := countingFleet{
		routeErr:  errors.New("connection refused"),
		reportErr: errors.New("connection refused"),
	}

	live, err := newRun(g, cfg, baselineEngine(t, g), broken, true, func() bool { return true },
		make(chan struct{}), logger)
	if err != nil {
		t.Fatalf("newRun: %v", err)
	}
	for i := 0; i < 5; i++ {
		live.advance()
	}
	live.report("settled")

	got := lines(t, &buf)
	// The first route failure, then the summary. No report line follows, because a
	// vehicle that never got a route never departs and never reports: the run is
	// empty from the first tick, which is exactly what the summary has to say.
	if len(got) < 2 {
		t.Fatalf("logged %d lines, want at least the first failure and the summary: %v", len(got), got)
	}

	summary := got[len(got)-1]
	if summary["level"] != "WARN" {
		t.Errorf("summary level = %v, want WARN for a run that sent nothing", summary["level"])
	}
	if msg, _ := summary["msg"].(string); !strings.Contains(msg, "without sending any traffic") {
		t.Errorf("msg = %v, want it to say nothing crossed the wire", summary["msg"])
	}
	if summary["api_route_calls"] != float64(0) {
		t.Errorf("api_route_calls = %v, want 0", summary["api_route_calls"])
	}
	if failures, _ := summary["api_route_failures"].(float64); failures == 0 {
		t.Error("the summary does not say the calls failed, so it reads as an idle run")
	}
}

// TestRunReport_CountsTheTrafficItSent is the other side: a run whose calls landed
// is reported at info, with the counts that prove it.
func TestRunReport_CountsTheTrafficItSent(t *testing.T) {
	g := testGraph(t)
	cfg := quickRunConfig()

	var buf bytes.Buffer
	logger := logBuffer(&buf)

	live, err := newRun(g, cfg, baselineEngine(t, g), countingFleet{}, true, func() bool { return true },
		make(chan struct{}), logger)
	if err != nil {
		t.Fatalf("newRun: %v", err)
	}
	for i := 0; i < 5; i++ {
		live.advance()
	}
	live.report("settled")

	got := lines(t, &buf)
	if len(got) != 1 {
		t.Fatalf("logged %d lines, want 1: %v", len(got), got)
	}
	if got[0]["level"] != "INFO" {
		t.Errorf("level = %v, want INFO for a run whose calls landed", got[0]["level"])
	}
	if calls, _ := got[0]["api_route_calls"].(float64); calls == 0 {
		t.Errorf("api_route_calls = %v, want the calls the run made", got[0]["api_route_calls"])
	}
	if got[0]["reason"] != "settled" {
		t.Errorf("reason = %v, want settled", got[0]["reason"])
	}
}

// TestRunReport_SaysNothingWhenThereIsNoWire keeps the in-process arm quiet: it has
// no API calls to account for, so claiming zero would read as a failure.
func TestRunReport_SaysNothingWhenThereIsNoWire(t *testing.T) {
	g := testGraph(t)
	cfg := quickRunConfig()

	var buf bytes.Buffer
	logger := logBuffer(&buf)

	// overWire is false: this is the in-process arm, which has no API calls to
	// account for, so claiming zero would read as a failure.
	live, err := newRun(g, cfg, baselineEngine(t, g), sim.NewEngineFleet(newEngine(t, g)), false,
		func() bool { return true }, make(chan struct{}), logger)
	if err != nil {
		t.Fatalf("newRun: %v", err)
	}
	for i := 0; i < 5; i++ {
		live.advance()
	}
	live.report("settled")

	got := lines(t, &buf)
	if len(got) != 1 {
		t.Fatalf("logged %d lines, want 1: %v", len(got), got)
	}
	if _, present := got[0]["api_route_calls"]; present {
		t.Errorf("an in-process run reported api_route_calls: %v", got[0])
	}
	if got[0]["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", got[0]["level"])
	}
}

// TestRunReport_IsSilentWithoutALogger keeps a nil logger from panicking, which is
// what a test that does not care about logging passes.
func TestRunReport_IsSilentWithoutALogger(t *testing.T) {
	g := testGraph(t)
	live, err := newRun(g, quickRunConfig(), baselineEngine(t, g), countingFleet{}, true,
		func() bool { return true }, make(chan struct{}), nil)
	if err != nil {
		t.Fatalf("newRun: %v", err)
	}

	live.report("settled") // must not panic
}

// TestSimStart_LogsTheRunItStarted covers the line that makes the rest readable: a
// finished run's numbers mean nothing without the request that produced them.
func TestSimStart_LogsTheRunItStarted(t *testing.T) {
	var buf bytes.Buffer
	s := testServer(t,
		WithEngine(newEngine(t, testGraph(t))),
		WithDemo(true),
		WithLogger(logBuffer(&buf)),
	)
	t.Cleanup(func() { s.stopRun(context.Background()) })

	if rec := postJSON(t, s, "/api/sim/start", `{"seed":11,"vehicles":5,"window_s":0}`); rec.Code != http.StatusAccepted {
		t.Fatalf("start = %d, want 202: %s", rec.Code, rec.Body)
	}

	var found bool
	for _, line := range lines(t, &buf) {
		if line["msg"] != "comparison run started" {
			continue
		}
		found = true
		if seed, _ := line["seed"].(float64); seed != 11 {
			t.Errorf("logged seed = %v, want 11", line["seed"])
		}
		if line["demo"] != true {
			t.Errorf("logged demo = %v, want true", line["demo"])
		}
		if _, present := line["request_id"]; !present {
			t.Error("the start line has no request_id, so it cannot be tied to the response")
		}
	}
	if !found {
		t.Fatalf("no start line was logged: %s", buf.String())
	}
}

// TestSimStart_LogsTheRefusal covers the line a refused caller cannot see: the
// response says a run is in progress, and the log says which.
func TestSimStart_LogsTheRefusal(t *testing.T) {
	var buf bytes.Buffer
	s := testServer(t,
		WithEngine(newEngine(t, testGraph(t))),
		WithDemo(true),
		WithLogger(logBuffer(&buf)),
	)
	t.Cleanup(func() { s.stopRun(context.Background()) })

	postJSON(t, s, "/api/sim/start", `{"seed":42,"vehicles":5,"window_s":0}`)
	if rec := postJSON(t, s, "/api/sim/start", `{"seed":43}`); rec.Code != http.StatusConflict {
		t.Fatalf("second start = %d, want 409: %s", rec.Code, rec.Body)
	}

	var found bool
	for _, line := range lines(t, &buf) {
		if line["msg"] != "refused a second start: a run is already going" {
			continue
		}
		found = true
		if seed, _ := line["running_seed"].(float64); seed != 42 {
			t.Errorf("running_seed = %v, want 42", line["running_seed"])
		}
	}
	if !found {
		t.Fatalf("the refusal was not logged: %s", buf.String())
	}
}
