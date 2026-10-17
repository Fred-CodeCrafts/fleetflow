package memory_test

import (
	"context"
	"errors"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/driver"
	"fleetflow/internal/driver/memory"
)

func TestCreateThenList(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	want := driver.Driver{ID: "a1", Name: "Ada Lovelace", Contact: "ada@example.com"}
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
	second := driver.Driver{ID: "a2", Name: "Grace Hopper"}
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
	d := driver.Driver{ID: "a1", Name: "Ada Lovelace"}

	if _, err := repo.Create(ctx, d); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := repo.Create(ctx, d)
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
		t.Errorf("List returned %d drivers after a rejected create, want 1", len(list))
	}
}

func TestCreateRejectsMissingID(t *testing.T) {
	repo := memory.New()

	_, err := repo.Create(context.Background(), driver.Driver{Name: "Ada Lovelace"})
	if err == nil {
		t.Fatal("Create accepted a driver with no id")
	}
	if kind := domain.KindOf(err); kind != domain.KindInternal {
		t.Errorf("Create kind = %s, want internal", kind)
	}
}

func TestListReturnsAFreshSlice(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	if _, err := repo.Create(ctx, driver.Driver{ID: "a1", Name: "Ada Lovelace"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	list[0].Name = "tampered"
	list = append(list, driver.Driver{ID: "a2"})

	again, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(again) != 1 || again[0].Name != "Ada Lovelace" {
		t.Errorf("List returned %+v after the caller mutated the first result, want the stored record", again)
	}
}

func TestMethodsHonourACancelledContext(t *testing.T) {
	repo := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := repo.Create(ctx, driver.Driver{ID: "a1", Name: "Ada"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Create error = %v, want context.Canceled", err)
	}
	if _, err := repo.List(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("List error = %v, want context.Canceled", err)
	}
	if _, err := repo.Assign(ctx, "a1", "v1"); !errors.Is(err, context.Canceled) {
		t.Errorf("Assign error = %v, want context.Canceled", err)
	}
}

func TestAssignGivesADriverAVehicle(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	if _, err := repo.Create(ctx, driver.Driver{ID: "a1", Name: "Ada Lovelace"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Assign(ctx, "a1", "v1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if got.VehicleID != "v1" {
		t.Errorf("Assign returned VehicleID %q, want v1", got.VehicleID)
	}

	// The change must live in the store, not only in the returned copy.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].VehicleID != "v1" {
		t.Errorf("List returned %+v, want the vehicle to be assigned", list)
	}

	// An empty vehicle takes it away again.
	got, err = repo.Assign(ctx, "a1", "")
	if err != nil {
		t.Fatalf("Assign unassign: %v", err)
	}
	if got.VehicleID != "" {
		t.Errorf("Assign returned VehicleID %q after unassigning, want empty", got.VehicleID)
	}
}

func TestAssignRejectsAnUnknownDriver(t *testing.T) {
	repo := memory.New()

	_, err := repo.Assign(context.Background(), "nobody", "v1")
	if err == nil {
		t.Fatal("Assign accepted an unknown driver")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Assign kind = %s, want not found", kind)
	}
}

func TestAssignRefusesAVehicleAnotherDriverHolds(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	for _, id := range []string{"a1", "a2"} {
		if _, err := repo.Create(ctx, driver.Driver{ID: id, Name: "Driver " + id}); err != nil {
			t.Fatalf("Create(%s): %v", id, err)
		}
	}
	if _, err := repo.Assign(ctx, "a1", "v1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	_, err := repo.Assign(ctx, "a2", "v1")
	if err == nil {
		t.Fatal("Assign let two drivers hold the same vehicle")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Assign kind = %s, want conflict", kind)
	}

	// A rejected assignment must leave both drivers as they were.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list[0].VehicleID != "v1" || list[1].VehicleID != "" {
		t.Errorf("List = %+v, want the rejected assignment to change nothing", list)
	}
}

// TestAssignToTheSameDriverIsNotAConflict covers the check that a driver does not
// collide with itself when it keeps the vehicle it already has.
func TestAssignToTheSameDriverIsNotAConflict(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	if _, err := repo.Create(ctx, driver.Driver{ID: "a1", Name: "Ada"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	for i := 0; i < 2; i++ {
		got, err := repo.Assign(ctx, "a1", "v1")
		if err != nil {
			t.Fatalf("Assign %d: %v", i, err)
		}
		if got.VehicleID != "v1" {
			t.Errorf("Assign %d returned VehicleID %q, want v1", i, got.VehicleID)
		}
	}
}
