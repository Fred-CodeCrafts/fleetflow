package simulation_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"fleetflow/internal/simulation"
	"fleetflow/internal/simulation/memory"
	"fleetflow/internal/simulation/sumo"
)

type executorFunc func(context.Context, sumo.Scenario) (sumo.Result, error)

func (f executorFunc) Run(ctx context.Context, scenario sumo.Scenario) (sumo.Result, error) {
	return f(ctx, scenario)
}

func TestStartPersistsCompletedSUMOResult(t *testing.T) {
	arrival := 20.0
	result := sumo.Result{
		SUMOVersion: "SUMO 1.27.1",
		Seed:        7,
		Trips:       []sumo.Trip{{ID: "fleetflow0", Status: sumo.TripCompleted, ArrivalS: &arrival}},
		Tracks: []sumo.Track{{VehicleID: "fleetflow0", Positions: []sumo.Position{
			{TimeS: 0, Longitude: 106.8, Latitude: -6.2},
			{TimeS: 5, Longitude: 106.81, Latitude: -6.21},
			{TimeS: 10, Longitude: 106.82, Latitude: -6.22},
			{TimeS: 15, Longitude: 106.83, Latitude: -6.23},
		}}},
	}
	service, err := simulation.NewService(context.Background(), memory.New(), executorFunc(func(context.Context, sumo.Scenario) (sumo.Result, error) {
		return result, nil
	}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Shutdown(context.Background())

	created, err := service.Start(context.Background(), sumo.Scenario{Seed: 7, Vehicles: 1, DepartureWindowS: 60})
	if err != nil {
		t.Fatal(err)
	}
	completed := waitForRun(t, service, created.ID, simulation.StatusCompleted)
	if completed.Result == nil || completed.Result.SUMOVersion != "SUMO 1.27.1" {
		t.Fatalf("completed run result = %#v", completed.Result)
	}
	if completed.StartedAt == nil || completed.FinishedAt == nil {
		t.Fatalf("run timestamps = started %v, finished %v", completed.StartedAt, completed.FinishedAt)
	}
	if completed.Result == nil || len(completed.Result.Deliveries) != 1 ||
		len(completed.Result.Deliveries[0].Stops) == 0 || len(completed.Result.Deliveries[0].Events) == 0 {
		t.Fatalf("completed run has no generated delivery history: %#v", completed.Result)
	}
}

func TestStartRejectsConcurrentScenario(t *testing.T) {
	started := make(chan struct{})
	service, err := simulation.NewService(context.Background(), memory.New(), executorFunc(func(ctx context.Context, _ sumo.Scenario) (sumo.Result, error) {
		close(started)
		<-ctx.Done()
		return sumo.Result{}, ctx.Err()
	}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.Start(context.Background(), sumo.Scenario{Seed: 1, Vehicles: 1, DepartureWindowS: 60})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := service.Start(context.Background(), sumo.Scenario{Seed: 2, Vehicles: 1, DepartureWindowS: 60}); err == nil {
		t.Fatal("Start() accepted a second concurrent scenario")
	}
	if err := service.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	failed := waitForRun(t, service, first.ID, simulation.StatusFailed)
	if failed.Error == "" {
		t.Fatal("failed run has no error detail")
	}
}

func TestStartRejectsInvalidScenario(t *testing.T) {
	service, err := simulation.NewService(context.Background(), memory.New(), executorFunc(func(context.Context, sumo.Scenario) (sumo.Result, error) {
		t.Fatal("executor called for invalid scenario")
		return sumo.Result{}, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Start(context.Background(), sumo.Scenario{})
	if err == nil {
		t.Fatal("Start() accepted an invalid scenario")
	}
	_ = service.Shutdown(context.Background())
}

func waitForRun(t *testing.T, service *simulation.Service, id string, status simulation.Status) simulation.Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := service.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == status {
			return run
		}
		time.Sleep(time.Millisecond)
	}
	run, err := service.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("run status = %q, want %q; error=%q", run.Status, status, run.Error)
	return simulation.Run{}
}
