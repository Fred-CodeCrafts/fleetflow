package vehicle_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/vehicle"
	"fleetflow/internal/vehicle/memory"
)

// newService returns a Service backed by a fresh in-memory repository, which is
// how DESIGN 3.7 asks services to be unit-tested.
func newService() *vehicle.Service { return vehicle.NewService(memory.New()) }

func TestServiceCreateStoresVehicle(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	got, err := svc.Create(ctx, vehicle.CreateInput{Name: "B 1234 XY", Type: "van", Status: "available"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == "" {
		t.Error("Create returned an empty id, want a generated one")
	}
	if got.Name != "B 1234 XY" || got.Type != "van" || got.Status != vehicle.StatusAvailable {
		t.Errorf("Create returned %+v, want the values it was given", got)
	}

	list, err := svc.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0] != got {
		t.Errorf("List returned %+v, want the created vehicle %+v", list, got)
	}
}

func TestServiceCreateRejectsInvalidInputAndSavesNothing(t *testing.T) {
	tests := []struct {
		name     string
		in       vehicle.CreateInput
		wantCode string
	}{
		{"empty name", vehicle.CreateInput{Type: "van", Status: "available"}, "vehicle_name_required"},
		{"blank name", vehicle.CreateInput{Name: "   ", Type: "van", Status: "available"}, "vehicle_name_required"},
		{"unknown status", vehicle.CreateInput{Name: "B 1", Type: "van", Status: "flying"}, "vehicle_status_unknown"},
		{"empty status", vehicle.CreateInput{Name: "B 1", Type: "van"}, "vehicle_status_unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService()
			ctx := context.Background()

			got, err := svc.Create(ctx, tt.in)
			if err == nil {
				t.Fatalf("Create accepted invalid input as %+v", got)
			}
			if kind := domain.KindOf(err); kind != domain.KindInvalid {
				t.Errorf("Create kind = %s, want invalid", kind)
			}
			if code, _, ok := domain.Public(err); !ok || code != tt.wantCode {
				t.Errorf("Create code = %q (ok=%v), want %q", code, ok, tt.wantCode)
			}

			// PRD US-1: invalid input saves nothing.
			list, err := svc.List(ctx, vehicle.ListFilter{})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(list) != 0 {
				t.Errorf("invalid input saved %d vehicle(s)", len(list))
			}
		})
	}
}

func TestServiceListReturnsCreatedVehiclesInCreationOrder(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	names := []string{"first", "second", "third"}
	for _, name := range names {
		if _, err := svc.Create(ctx, vehicle.CreateInput{Name: name, Status: "available"}); err != nil {
			t.Fatalf("Create(%q): %v", name, err)
		}
	}

	list, err := svc.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(names) {
		t.Fatalf("List returned %d vehicles, want %d", len(list), len(names))
	}
	for i, name := range names {
		if list[i].Name != name {
			t.Errorf("list[%d].Name = %q, want %q", i, list[i].Name, name)
		}
	}
}

func TestServiceListFiltersByStatus(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	for i, status := range []string{"available", "on_trip", "available", "offline"} {
		in := vehicle.CreateInput{Name: fmt.Sprintf("vehicle-%d", i), Status: status}
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("Create(%s): %v", status, err)
		}
	}

	list, err := svc.List(ctx, vehicle.ByStatus(vehicle.StatusAvailable))
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

func TestServiceListRejectsUnknownStatusFilter(t *testing.T) {
	svc := newService()

	_, err := svc.List(context.Background(), vehicle.ListFilter{Status: ptrStatus(vehicle.Status(42))})
	if err == nil {
		t.Fatal("List accepted an out-of-range status filter")
	}
	if kind := domain.KindOf(err); kind != domain.KindInvalid {
		t.Errorf("List kind = %s, want invalid", kind)
	}
}

func TestServiceIDsAreUnique(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	const count = 200
	seen := make(map[string]bool, count)
	for i := 0; i < count; i++ {
		got, err := svc.Create(ctx, vehicle.CreateInput{Name: fmt.Sprintf("vehicle-%d", i), Status: "available"})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		if got.ID == "" {
			t.Fatalf("Create %d returned an empty id", i)
		}
		if seen[got.ID] {
			t.Fatalf("Create %d repeated id %q", i, got.ID)
		}
		seen[got.ID] = true
	}

	list, err := svc.List(ctx, vehicle.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != count {
		t.Errorf("List returned %d vehicles, want %d", len(list), count)
	}
}

// stubRepo is a Repository that always fails, to check how the service
// classifies a storage failure without needing a database.
type stubRepo struct{ err error }

func (s stubRepo) Create(context.Context, vehicle.Vehicle) (vehicle.Vehicle, error) {
	return vehicle.Vehicle{}, s.err
}

func (s stubRepo) List(context.Context, vehicle.ListFilter) ([]vehicle.Vehicle, error) {
	return nil, s.err
}

func TestServiceHidesRawStorageError(t *testing.T) {
	raw := errors.New("pq: dial tcp 10.0.0.7:5432: connection refused")
	svc := vehicle.NewService(stubRepo{err: raw})

	_, err := svc.Create(context.Background(), vehicle.CreateInput{Name: "B 1", Status: "offline"})
	if err == nil {
		t.Fatal("Create succeeded although the repository failed")
	}
	if kind := domain.KindOf(err); kind != domain.KindInternal {
		t.Errorf("Create kind = %s, want internal", kind)
	}
	code, message, ok := domain.Public(err)
	if !ok || code != "vehicle_create_failed" {
		t.Errorf("Create code = %q (ok=%v), want vehicle_create_failed", code, ok)
	}
	if strings.Contains(message, "10.0.0.7") {
		t.Errorf("Create leaked the internal cause in %q", message)
	}
	if !errors.Is(err, raw) {
		t.Error("Create dropped the cause, so logs cannot show why storage failed")
	}
}

func TestServiceKeepsClassifiedRepositoryError(t *testing.T) {
	conflict := domain.Conflict("vehicle_exists", "vehicle dupe already exists")
	svc := vehicle.NewService(stubRepo{err: conflict})

	_, err := svc.Create(context.Background(), vehicle.CreateInput{Name: "B 1", Status: "offline"})
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
	if code, _, _ := domain.Public(err); code != "vehicle_exists" {
		t.Errorf("Create code = %q, want vehicle_exists", code)
	}
}

func TestServiceReportsCancelledRequestAsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := newService()
	_, err := svc.Create(ctx, vehicle.CreateInput{Name: "B 1", Status: "available"})
	if err == nil {
		t.Fatal("Create succeeded with a cancelled context")
	}
	if kind := domain.KindOf(err); kind != domain.KindUnavailable {
		t.Errorf("Create kind = %s, want unavailable", kind)
	}
	if code, _, _ := domain.Public(err); code != "request_cancelled" {
		t.Errorf("Create code = %q, want request_cancelled", code)
	}
}

// ptrStatus returns a pointer to a status, for filters that carry one.
func ptrStatus(s vehicle.Status) *vehicle.Status { return &s }
