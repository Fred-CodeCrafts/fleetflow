package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"fleetflow/internal/simulation"
	"fleetflow/internal/simulation/postgres"
	"fleetflow/internal/simulation/sumo"
	"fleetflow/internal/store/storetest"
)

func TestRepositoryPersistsRunAndReturnsDetailedResult(t *testing.T) {
	pool := storetest.Pool(t)
	storetest.Reset(t, pool)
	repo := postgres.New(pool)
	ctx := context.Background()
	run := simulation.Run{
		ID:        "sim_postgres_test",
		Status:    simulation.StatusQueued,
		Scenario:  sumo.Scenario{Seed: 42, Vehicles: 2, DepartureWindowS: 120},
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatal(err)
	}

	finished := time.Now().UTC()
	run.Status = simulation.StatusCompleted
	run.StartedAt = &finished
	run.FinishedAt = &finished
	run.Result = &sumo.Result{
		SUMOVersion: "SUMO test",
		Seed:        42,
		Trips:       []sumo.Trip{{ID: "fleetflow0", Status: sumo.TripCompleted}},
	}
	if err := repo.Update(ctx, run); err != nil {
		t.Fatal(err)
	}

	found, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Status != simulation.StatusCompleted || found.Result == nil ||
		found.Result.SUMOVersion != run.Result.SUMOVersion || len(found.Result.Trips) != 1 {
		t.Fatalf("Get() returned incomplete run: %#v", found)
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != run.ID || list[0].Result != nil {
		t.Fatalf("List() = %#v, want metadata without the detailed result", list)
	}
}

func TestRepositoryReturnsNotFoundSentinel(t *testing.T) {
	pool := storetest.Pool(t)
	storetest.Reset(t, pool)
	_, err := postgres.New(pool).Get(context.Background(), "absent")
	if !errors.Is(err, simulation.ErrRunNotFound) {
		t.Fatalf("Get() error = %v, want ErrRunNotFound", err)
	}
}
