package postgres_test

import (
	"context"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/trip"
)

func TestSetStatusMovesTheTripForward(t *testing.T) {
	repo, _ := newRepository(t)
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned})

	got, err := repo.SetStatus(context.Background(), "t1", trip.StatusInProgress)
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if got.Status != trip.StatusInProgress {
		t.Errorf("SetStatus returned %s, want in_progress", got.Status)
	}

	// The stored row is what changed, not only the returned value.
	list, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Status != trip.StatusInProgress {
		t.Errorf("List returned %+v, want the trip in progress", list)
	}
}

// TestSetStatusKeepsTheVehicleMovingATripForward must not be an assignment in
// disguise: the vehicle carrying the trip stays on it.
func TestSetStatusKeepsTheVehicle(t *testing.T) {
	repo, pool := newRepository(t)
	insertVehicle(t, pool, "v1")
	mustCreate(t, repo, trip.Trip{
		ID: "t1", Origin: "A", Destination: "B",
		VehicleID: "v1", Status: trip.StatusPlanned,
	})

	got, err := repo.SetStatus(context.Background(), "t1", trip.StatusInProgress)
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if got.VehicleID != "v1" {
		t.Errorf("SetStatus returned VehicleID %q, want v1", got.VehicleID)
	}
}

func TestSetStatusRefusesAMoveBackwards(t *testing.T) {
	repo, _ := newRepository(t)
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusCompleted})

	_, err := repo.SetStatus(context.Background(), "t1", trip.StatusPlanned)
	if err == nil {
		t.Fatal("SetStatus un-finished a completed trip")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("SetStatus kind = %s, want conflict", kind)
	}
	if code, _, _ := domain.Public(err); code != "trip_status_regressed" {
		t.Errorf("SetStatus code = %q, want trip_status_regressed", code)
	}

	// A refused move must leave the stored row alone.
	list, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Status != trip.StatusCompleted {
		t.Errorf("List returned %+v, want the trip still completed", list)
	}
}

func TestSetStatusSettingTheSameStatusIsAllowed(t *testing.T) {
	repo, _ := newRepository(t)
	mustCreate(t, repo, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned})

	got, err := repo.SetStatus(context.Background(), "t1", trip.StatusPlanned)
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if got.Status != trip.StatusPlanned {
		t.Errorf("SetStatus returned %s, want planned", got.Status)
	}
}

func TestSetStatusUnknownTripIsNotFound(t *testing.T) {
	repo, _ := newRepository(t)

	_, err := repo.SetStatus(context.Background(), "nobody", trip.StatusInProgress)
	if err == nil {
		t.Fatal("SetStatus accepted a trip that does not exist")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("SetStatus kind = %s, want not found", kind)
	}
	if code, _, _ := domain.Public(err); code != "trip_not_found" {
		t.Errorf("SetStatus code = %q, want trip_not_found", code)
	}
}
