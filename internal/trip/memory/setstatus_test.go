package memory_test

import (
	"context"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/trip"
	"fleetflow/internal/trip/memory"
)

func TestSetStatusMovesATripForward(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	if _, err := repo.Create(ctx, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.SetStatus(ctx, "t1", trip.StatusInProgress)
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if got.Status != trip.StatusInProgress {
		t.Errorf("SetStatus returned %s, want in_progress", got.Status)
	}

	// The stored record is what changed, not only the returned copy.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Status != trip.StatusInProgress {
		t.Errorf("List returned %+v, want the trip in progress", list)
	}
}

func TestSetStatusRefusesAMoveBackwards(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	if _, err := repo.Create(ctx, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusCompleted}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err := repo.SetStatus(ctx, "t1", trip.StatusPlanned)
	if err == nil {
		t.Fatal("SetStatus un-finished a completed trip")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("SetStatus kind = %s, want conflict", kind)
	}
	if code, _, _ := domain.Public(err); code != "trip_status_regressed" {
		t.Errorf("SetStatus code = %q, want trip_status_regressed", code)
	}

	// A refused move must leave the stored status alone.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Status != trip.StatusCompleted {
		t.Errorf("List returned %+v, want the trip still completed", list)
	}
}

func TestSetStatusSettingTheSameStatusChangesNothing(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	if _, err := repo.Create(ctx, trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.SetStatus(ctx, "t1", trip.StatusPlanned)
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if got.Status != trip.StatusPlanned {
		t.Errorf("SetStatus returned %s, want planned", got.Status)
	}
}

func TestSetStatusRejectsAnUnknownTrip(t *testing.T) {
	repo := memory.New()

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
