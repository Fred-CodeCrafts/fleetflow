package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// object reads a nested JSON object out of a decoded body, failing the test when
// the key is missing or is not an object. The metrics response is nested enough
// that doing this by hand at each use would bury the assertions.
func object(t *testing.T, body map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := body[key]
	if !ok {
		t.Fatalf("body has no %q: %v", key, body)
	}
	nested, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%q is %T, want an object: %v", key, value, value)
	}
	return nested
}

// number reads a numeric field out of a decoded object.
func number(t *testing.T, obj map[string]any, key string) float64 {
	t.Helper()

	value, ok := obj[key]
	if !ok {
		t.Fatalf("object has no %q: %v", key, obj)
	}
	n, ok := value.(float64)
	if !ok {
		t.Fatalf("%q is %T, want a number: %v", key, value, value)
	}
	return n
}

func TestMetrics_ReportsTheEngineWithoutARun(t *testing.T) {
	s := simServer(t)

	rec := getJSON(t, s, "/api/metrics")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	body := decodeBody(t, rec)
	engine := object(t, body, "engine")
	if version := number(t, engine, "metric_version"); version < 1 {
		t.Errorf("metric_version = %v, want at least the initial snapshot", version)
	}
	if age := number(t, engine, "metric_age_s"); age < 0 {
		t.Errorf("metric_age_s = %v, want a non-negative age", age)
	}
	if _, ok := engine["customize_ms"]; !ok {
		t.Error("the engine block carries no customization time")
	}
	if run, ok := body["run"]; !ok || run != nil {
		t.Errorf("run = %v, want null when nothing is going", run)
	}
}

func TestMetrics_ReportsBothArmsOfTheRun(t *testing.T) {
	s := simServer(t)
	postJSON(t, s, "/api/sim/start", `{"seed":11,"vehicles":4,"fleet_share":1,"window_s":0,"step_s":1}`)

	rec := getJSON(t, s, "/api/metrics")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	body := decodeBody(t, rec)
	run := object(t, body, "run")
	if seed := number(t, run, "seed"); seed != 11 {
		t.Errorf("seed = %v, want 11", seed)
	}
	if vehicles := number(t, run, "vehicles"); vehicles != 4 {
		t.Errorf("vehicles = %v, want 4", vehicles)
	}

	// Both arms ran the same demand. The figures move as the run goes, so the
	// demand is what a single request can be pinned to.
	baseline := object(t, run, "baseline")
	fleetflow := object(t, run, "fleet_flow")
	if got := number(t, baseline, "total_vehicles"); got != 4 {
		t.Errorf("baseline total_vehicles = %v, want 4", got)
	}
	if got := number(t, fleetflow, "total_vehicles"); got != 4 {
		t.Errorf("fleet_flow total_vehicles = %v, want 4: the arms must run the same demand", got)
	}

	// The fairness check is reported per arm, not merged into one number.
	for name, arm := range map[string]map[string]any{"baseline": baseline, "fleet_flow": fleetflow} {
		fairness := object(t, arm, "fairness")
		if _, ok := fairness["mean_time_lost_s"]; !ok {
			t.Errorf("%s carries no fairness check: %v", name, fairness)
		}
		if _, ok := fairness["mean_fleet_trip_s"]; !ok {
			t.Errorf("%s carries no fleet trip time to compare: %v", name, fairness)
		}
	}
}

func TestMetrics_ForgetsTheRunAfterAReset(t *testing.T) {
	s := simServer(t)
	postJSON(t, s, "/api/sim/start", `{"vehicles":4,"window_s":600}`)
	postJSON(t, s, "/api/sim/reset", "")

	rec := getJSON(t, s, "/api/metrics")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if run := decodeBody(t, rec)["run"]; run != nil {
		t.Errorf("run = %v after a reset, want null", run)
	}
}

func TestMetrics_WithoutAnEngineIsUnavailable(t *testing.T) {
	s := testServer(t)

	rec := getJSON(t, s, "/api/metrics")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if got := decodeBody(t, rec)["code"]; got != "engine_unavailable" {
		t.Errorf("code = %v, want engine_unavailable", got)
	}
}

func TestMetrics_IsOpenToTheDashboard(t *testing.T) {
	s := simServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 without an API key", rec.Code)
	}
}

func TestMetrics_RejectsOtherMethods(t *testing.T) {
	s := simServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/metrics", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
