package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"fleetflow/internal/simulation"
	simulationmemory "fleetflow/internal/simulation/memory"
	"fleetflow/internal/simulation/sumo"
)

type immediateSUMOExecutor struct{}

func (immediateSUMOExecutor) Run(context.Context, sumo.Scenario) (sumo.Result, error) {
	return sumo.Result{
		SUMOVersion: "SUMO test",
		Trips:       []sumo.Trip{{ID: "fleetflow0", Status: sumo.TripCompleted}},
	}, nil
}

func TestSimulationRunEndpoints(t *testing.T) {
	service, err := simulation.NewService(context.Background(), simulationmemory.New(),
		immediateSUMOExecutor{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Shutdown(context.Background())
	server := testServer(t, WithSimulation(service))

	created := postJSON(t, server, "/api/simulation/runs",
		`{"seed":42,"vehicles":1,"departure_window_s":60}`)
	if created.Code != http.StatusAccepted {
		t.Fatalf("POST run status = %d, want 202: %s", created.Code, created.Body.String())
	}
	run := decodeBody(t, created)
	id := field(t, run, "id")
	if created.Header().Get("Location") != "/api/simulation/runs/"+id {
		t.Fatalf("Location = %q", created.Header().Get("Location"))
	}

	var latest map[string]any
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec := getJSON(t, server, "/api/simulation/runs/"+id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET run status = %d: %s", rec.Code, rec.Body.String())
		}
		latest = decodeBody(t, rec)
		if latest["status"] == string(simulation.StatusCompleted) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if latest["status"] != string(simulation.StatusCompleted) {
		t.Fatalf("run status = %v, want completed", latest["status"])
	}
	if latest["result"] == nil {
		t.Fatal("completed run has no SUMO result")
	}

	list := getJSON(t, server, "/api/simulation/runs")
	if list.Code != http.StatusOK || len(listOf(t, decodeBody(t, list), "runs")) != 1 {
		t.Fatalf("GET runs = %d %s", list.Code, list.Body.String())
	}
}

func TestSimulationRunEndpointsReportUnavailableAndInvalidInputs(t *testing.T) {
	server := testServer(t)
	unavailable := postJSON(t, server, "/api/simulation/runs",
		`{"seed":1,"vehicles":1,"departure_window_s":60}`)
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured simulation status = %d, want 503", unavailable.Code)
	}

	service, err := simulation.NewService(context.Background(), simulationmemory.New(),
		immediateSUMOExecutor{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Shutdown(context.Background())
	server = testServer(t, WithSimulation(service))

	invalid := postJSON(t, server, "/api/simulation/runs",
		`{"seed":1,"vehicles":0,"departure_window_s":60}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid scenario status = %d, want 400: %s", invalid.Code, invalid.Body.String())
	}
}
