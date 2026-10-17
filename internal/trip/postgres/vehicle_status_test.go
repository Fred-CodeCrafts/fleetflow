package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/trip"
)

// vehicleStatus reads a vehicle's stored status directly, so a test can check
// what the trip rules actually wrote rather than what a call returned.
func vehicleStatus(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()

	var status string
	if err := pool.QueryRow(context.Background(),
		`select status from vehicles where id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("reading vehicle %s: %v", id, err)
	}
	return status
}

// TestCreateMarksTheVehicleOnTrip is the first half of keeping the two in step: a
// vehicle that a trip has taken says so, which is what stops a second trip being
// offered it.
func TestCreateMarksTheVehicleOnTrip(t *testing.T) {
	repo, pool := newRepository(t)
	insertVehicle(t, pool, "v1")

	mustCreate(t, repo, trip.Trip{
		ID: "t1", Origin: "A", Destination: "B",
		VehicleID: "v1", Status: trip.StatusPlanned,
	})

	if got := vehicleStatus(t, pool, "v1"); got != "on_trip" {
		t.Errorf("vehicle status = %q, want on_trip", got)
	}
}

// TestCompletingATripFreesTheVehicle is the other half, and the one T39a turns
// on: finishing is what makes a vehicle available again.
func TestCompletingATripFreesTheVehicle(t *testing.T) {
	repo, pool := newRepository(t)
	insertVehicle(t, pool, "v1")
	mustCreate(t, repo, trip.Trip{
		ID: "t1", Origin: "A", Destination: "B",
		VehicleID: "v1", Status: trip.StatusPlanned,
	})

	if _, err := repo.SetStatus(context.Background(), "t1", trip.StatusCompleted); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if got := vehicleStatus(t, pool, "v1"); got != "available" {
		t.Errorf("vehicle status = %q, want available once the trip is completed", got)
	}
}

func TestAssigningAndUnassigningKeepTheVehicleInStep(t *testing.T) {
	repo, pool := newRepository(t)
	insertVehicle(t, pool, "v1")
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned})

	if _, err := repo.Assign(context.Background(), "t1", "v1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if got := vehicleStatus(t, pool, "v1"); got != "on_trip" {
		t.Errorf("vehicle status after assigning = %q, want on_trip", got)
	}

	if _, err := repo.Assign(context.Background(), "t1", ""); err != nil {
		t.Fatalf("unassigning: %v", err)
	}
	if got := vehicleStatus(t, pool, "v1"); got != "available" {
		t.Errorf("vehicle status after unassigning = %q, want available", got)
	}
}

func TestMovingATripToAnotherVehicleFreesTheFirst(t *testing.T) {
	repo, pool := newRepository(t)
	insertVehicle(t, pool, "v1")
	insertVehicle(t, pool, "v2")
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned})

	if _, err := repo.Assign(context.Background(), "t1", "v1"); err != nil {
		t.Fatalf("Assign v1: %v", err)
	}
	if _, err := repo.Assign(context.Background(), "t1", "v2"); err != nil {
		t.Fatalf("Assign v2: %v", err)
	}

	if got := vehicleStatus(t, pool, "v1"); got != "available" {
		t.Errorf("the vehicle the trip left is %q, want available", got)
	}
	if got := vehicleStatus(t, pool, "v2"); got != "on_trip" {
		t.Errorf("the vehicle the trip took is %q, want on_trip", got)
	}
}

// TestReSettingACompletedTripDoesNotFreeAnotherTripsVehicle guards the guard. A
// completed trip keeps its vehicle_id, so a retried completion must not release a
// vehicle that a later trip has since taken.
func TestReSettingACompletedTripDoesNotFreeAnotherTripsVehicle(t *testing.T) {
	repo, pool := newRepository(t)
	insertVehicle(t, pool, "v1")
	mustCreate(t, repo, trip.Trip{
		ID: "t1", Origin: "A", Destination: "B",
		VehicleID: "v1", Status: trip.StatusPlanned,
	})
	if _, err := repo.SetStatus(context.Background(), "t1", trip.StatusCompleted); err != nil {
		t.Fatalf("completing t1: %v", err)
	}

	// t1 released v1, so a second trip may take it.
	mustCreate(t, repo, trip.Trip{
		ID: "t2", Origin: "C", Destination: "D",
		VehicleID: "v1", Status: trip.StatusPlanned,
	})

	if _, err := repo.SetStatus(context.Background(), "t1", trip.StatusCompleted); err != nil {
		t.Fatalf("re-setting t1: %v", err)
	}
	if got := vehicleStatus(t, pool, "v1"); got != "on_trip" {
		t.Errorf("vehicle status = %q, want on_trip: t2 is still carrying it", got)
	}
}
