package memory_test

import (
	"context"
	"errors"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/vehicle"
	"fleetflow/internal/vehicle/memory"
)

func TestCreateThenList(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	want := vehicle.Vehicle{ID: "a1", Name: "B 1", Type: "van", Status: vehicle.StatusAvailable}
	stored, err := repo.Create(ctx, want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored != want {
		t.Errorf("Create returned %+v, want %+v", stored, want)
	}

	list, err := repo.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0] != want {
		t.Errorf("List returned %+v, want just %+v", list, want)
	}

	// Creation order is the order List returns.
	second := vehicle.Vehicle{ID: "a2", Name: "B 2", Status: vehicle.StatusOffline}
	if _, err := repo.Create(ctx, second); err != nil {
		t.Fatalf("Create: %v", err)
	}
	list, err = repo.List(ctx, vehicle.ListFilter{})
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
	v := vehicle.Vehicle{ID: "a1", Name: "B 1", Status: vehicle.StatusAvailable}

	if _, err := repo.Create(ctx, v); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := repo.Create(ctx, v)
	if err == nil {
		t.Fatal("Create accepted a duplicate id")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}

	list, err := repo.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List returned %d vehicles after a rejected create, want 1", len(list))
	}
}

func TestCreateRejectsMissingID(t *testing.T) {
	repo := memory.New()

	_, err := repo.Create(context.Background(), vehicle.Vehicle{Name: "B 1", Status: vehicle.StatusAvailable})
	if err == nil {
		t.Fatal("Create accepted a vehicle with no id")
	}
	if kind := domain.KindOf(err); kind != domain.KindInternal {
		t.Errorf("Create kind = %s, want internal", kind)
	}
}

func TestListFiltersByStatus(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	for i, status := range []vehicle.Status{vehicle.StatusAvailable, vehicle.StatusOnTrip, vehicle.StatusAvailable} {
		v := vehicle.Vehicle{ID: string(rune('a' + i)), Status: status}
		if _, err := repo.Create(ctx, v); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	list, err := repo.List(ctx, vehicle.ByStatus(vehicle.StatusAvailable))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List(available) returned %d vehicles, want 2", len(list))
	}
	for _, v := range list {
		if v.Status != vehicle.StatusAvailable {
			t.Errorf("List(available) returned a %s vehicle", v.Status)
		}
	}
}

func TestListReturnsAFreshSlice(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	if _, err := repo.Create(ctx, vehicle.Vehicle{ID: "a1", Name: "B 1", Status: vehicle.StatusAvailable}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := repo.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	list[0].Name = "tampered"
	list = append(list, vehicle.Vehicle{ID: "a2"})

	again, err := repo.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(again) != 1 || again[0].Name != "B 1" {
		t.Errorf("List returned %+v after the caller mutated the first result, want the stored record", again)
	}
}

func TestProfilePointersDoNotExposeStoredState(t *testing.T) {
	repo := memory.New()
	year, capacity := 2022, 1250.5
	created, err := repo.Create(context.Background(), vehicle.Vehicle{
		ID: "v1", Name: "Fleet van", Year: &year, Capacity: &capacity,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	*created.Year = 1999
	*created.Capacity = 0

	list, err := repo.List(context.Background(), vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	*list[0].Year = 2000
	*list[0].Capacity = 0
	again, err := repo.List(context.Background(), vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List after mutation: %v", err)
	}
	if *again[0].Year != year || *again[0].Capacity != capacity {
		t.Errorf("stored profile changed through returned pointers: %+v", again[0])
	}
}

func TestMethodsHonourACancelledContext(t *testing.T) {
	repo := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := repo.Create(ctx, vehicle.Vehicle{ID: "a1", Status: vehicle.StatusAvailable}); !errors.Is(err, context.Canceled) {
		t.Errorf("Create error = %v, want context.Canceled", err)
	}
	if _, err := repo.List(ctx, vehicle.ListFilter{}); !errors.Is(err, context.Canceled) {
		t.Errorf("List error = %v, want context.Canceled", err)
	}
}
