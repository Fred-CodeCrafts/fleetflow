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
}
