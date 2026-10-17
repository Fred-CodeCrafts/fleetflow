package memory_test

import (
	"context"
	"errors"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/trip"
	"fleetflow/internal/trip/memory"
)

func TestCreateThenList(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	want := trip.Trip{ID: "a1", Origin: "A", Destination: "B", VehicleID: "v1", Status: trip.StatusPlanned}
	stored, err := repo.Create(ctx, want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored != want {
		t.Errorf("Create returned %+v, want %+v", stored, want)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0] != want {
		t.Errorf("List returned %+v, want just %+v", list, want)
	}

	// Creation order is the order List returns.
	second := trip.Trip{ID: "a2", Origin: "C", Destination: "D", Status: trip.StatusCompleted}
	if _, err := repo.Create(ctx, second); err != nil {
		t.Fatalf("Create: %v", err)
	}
	list, err = repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID != "a1" || list[1].ID != "a2" {
		t.Errorf("List returned %+v, want a1 then a2", list)
	}
}

func TestCreateRejectsDuplicateID(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	tr := trip.Trip{ID: "a1", Origin: "A", Destination: "B", Status: trip.StatusPlanned}

	if _, err := repo.Create(ctx, tr); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := repo.Create(ctx, tr)
	if err == nil {
		t.Fatal("Create accepted a duplicate id")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List returned %d trips after a rejected create, want 1", len(list))
	}
}

func TestCreateRejectsMissingID(t *testing.T) {
	repo := memory.New()

	_, err := repo.Create(context.Background(), trip.Trip{Origin: "A", Destination: "B", Status: trip.StatusPlanned})
	if err == nil {
		t.Fatal("Create accepted a trip with no id")
	}
	if kind := domain.KindOf(err); kind != domain.KindInternal {
		t.Errorf("Create kind = %s, want internal", kind)
	}
}

func TestListReturnsAFreshSlice(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	tr := trip.Trip{ID: "a1", Origin: "A", Destination: "B", Status: trip.StatusPlanned}
	if _, err := repo.Create(ctx, tr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	list[0].Origin = "tampered"
	list = append(list, trip.Trip{ID: "a2"})

	again, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(again) != 1 || again[0].Origin != "A" {
		t.Errorf("List returned %+v after the caller mutated the first result, want the stored record", again)
	}
}

func TestMethodsHonourACancelledContext(t *testing.T) {
	repo := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tr := trip.Trip{ID: "a1", Origin: "A", Destination: "B", Status: trip.StatusPlanned}
	if _, err := repo.Create(ctx, tr); !errors.Is(err, context.Canceled) {
		t.Errorf("Create error = %v, want context.Canceled", err)
	}
	if _, err := repo.List(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("List error = %v, want context.Canceled", err)
	}
	if _, err := repo.Assign(ctx, "a1", "v1"); !errors.Is(err, context.Canceled) {
		t.Errorf("Assign error = %v, want context.Canceled", err)
	}
}

func TestAssignPutsATripOnAVehicle(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	tr := trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusPlanned}
	if _, err := repo.Create(ctx, tr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Assign(ctx, "t1", "v1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if got.VehicleID != "v1" {
		t.Errorf("Assign returned VehicleID %q, want v1", got.VehicleID)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].VehicleID != "v1" {
		t.Errorf("List returned %+v, want the assignment to be stored", list)
	}

	// An empty vehicle takes the trip off it again.
	if got, err = repo.Assign(ctx, "t1", ""); err != nil {
		t.Fatalf("Assign unassign: %v", err)
	}
	if got.VehicleID != "" {
		t.Errorf("Assign returned VehicleID %q after unassigning, want empty", got.VehicleID)
	}
}

func TestAssignRejectsAnUnknownTrip(t *testing.T) {
	repo := memory.New()

	_, err := repo.Assign(context.Background(), "nobody", "v1")
	if err == nil {
		t.Fatal("Assign accepted an unknown trip")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Assign kind = %s, want not found", kind)
	}
}

func TestAssignRefusesAVehicleAlreadyOnAnotherTrip(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	for _, id := range []string{"t1", "t2"} {
		tr := trip.Trip{ID: id, Origin: "A", Destination: "B", Status: trip.StatusPlanned}
		if _, err := repo.Create(ctx, tr); err != nil {
			t.Fatalf("Create(%s): %v", id, err)
		}
	}
	if _, err := repo.Assign(ctx, "t1", "v1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	_, err := repo.Assign(ctx, "t2", "v1")
	if err == nil {
		t.Fatal("Assign put one vehicle on two unfinished trips")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Assign kind = %s, want conflict", kind)
	}

	// Nothing may have moved.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list[0].VehicleID != "v1" || list[1].VehicleID != "" {
		t.Errorf("List = %+v, want the rejected assignment to change nothing", list)
	}
}

// TestAssignLetsACompletedTripReleaseItsVehicle covers the other half of
// availability: a finished trip does not keep a vehicle busy.
func TestAssignLetsACompletedTripReleaseItsVehicle(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	done := trip.Trip{ID: "t1", Origin: "A", Destination: "B", VehicleID: "v1", Status: trip.StatusCompleted}
	next := trip.Trip{ID: "t2", Origin: "C", Destination: "D", Status: trip.StatusPlanned}
	for _, tr := range []trip.Trip{done, next} {
		if _, err := repo.Create(ctx, tr); err != nil {
			t.Fatalf("Create(%s): %v", tr.ID, err)
		}
	}

	if _, err := repo.Assign(ctx, "t2", "v1"); err != nil {
		t.Errorf("Assign refused a vehicle a finished trip had released: %v", err)
	}
}

func TestAssignRefusesToReassignACompletedTrip(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	done := trip.Trip{ID: "t1", Origin: "A", Destination: "B", Status: trip.StatusCompleted}
	if _, err := repo.Create(ctx, done); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err := repo.Assign(ctx, "t1", "v1")
	if err == nil {
		t.Fatal("Assign put a vehicle on a completed trip")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Assign kind = %s, want conflict", kind)
	}
}
