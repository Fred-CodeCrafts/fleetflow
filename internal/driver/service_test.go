package driver_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/driver"
	"fleetflow/internal/driver/memory"
)

// newService returns a Service backed by a fresh in-memory repository, which is
// how DESIGN 3.7 asks services to be unit-tested.
func newService() *driver.Service { return driver.NewService(memory.New()) }

func TestServiceCreateStoresDriver(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	got, err := svc.Create(ctx, driver.CreateInput{Name: "Ada Lovelace", Contact: "ada@example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == "" {
		t.Error("Create returned an empty id, want a generated one")
	}
	if got.Name != "Ada Lovelace" || got.Contact != "ada@example.com" {
		t.Errorf("Create returned %+v, want the values it was given", got)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0] != got {
		t.Errorf("List returned %+v, want the created driver %+v", list, got)
	}
}

func TestServiceCreateRejectsBlankNameAndSavesNothing(t *testing.T) {
	tests := []struct {
		name string
		in   driver.CreateInput
	}{
		{"empty name", driver.CreateInput{Contact: "ada@example.com"}},
		{"blank name", driver.CreateInput{Name: "   ", Contact: "ada@example.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService()
			ctx := context.Background()

			got, err := svc.Create(ctx, tt.in)
			if err == nil {
				t.Fatalf("Create accepted blank input as %+v", got)
			}
			if kind := domain.KindOf(err); kind != domain.KindInvalid {
				t.Errorf("Create kind = %s, want invalid", kind)
			}
			if code, _, ok := domain.Public(err); !ok || code != "driver_name_required" {
				t.Errorf("Create code = %q (ok=%v), want driver_name_required", code, ok)
			}

			list, err := svc.List(ctx)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(list) != 0 {
				t.Errorf("invalid input saved %d driver(s)", len(list))
			}
		})
	}
}

func TestServiceListReturnsCreatedDriversInCreationOrder(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	names := []string{"first", "second", "third"}
	for _, name := range names {
		if _, err := svc.Create(ctx, driver.CreateInput{Name: name}); err != nil {
			t.Fatalf("Create(%q): %v", name, err)
		}
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(names) {
		t.Fatalf("List returned %d drivers, want %d", len(list), len(names))
	}
	for i, name := range names {
		if list[i].Name != name {
			t.Errorf("list[%d].Name = %q, want %q", i, list[i].Name, name)
		}
	}
}

func TestServiceIDsAreUnique(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	const count = 200
	seen := make(map[string]bool, count)
	for i := 0; i < count; i++ {
		got, err := svc.Create(ctx, driver.CreateInput{Name: fmt.Sprintf("driver-%d", i)})
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

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != count {
		t.Errorf("List returned %d drivers, want %d", len(list), count)
	}
}

// stubRepo is a Repository that always fails, to check how the service
// classifies a storage failure without needing a database.
type stubRepo struct{ err error }

func (s stubRepo) Create(context.Context, driver.Driver) (driver.Driver, error) {
	return driver.Driver{}, s.err
}

func (s stubRepo) List(context.Context) ([]driver.Driver, error) {
	return nil, s.err
}

func TestServiceHidesRawStorageError(t *testing.T) {
	raw := errors.New("pq: dial tcp 10.0.0.7:5432: connection refused")
	svc := driver.NewService(stubRepo{err: raw})

	_, err := svc.Create(context.Background(), driver.CreateInput{Name: "Ada"})
	if err == nil {
		t.Fatal("Create succeeded although the repository failed")
	}
	if kind := domain.KindOf(err); kind != domain.KindInternal {
		t.Errorf("Create kind = %s, want internal", kind)
	}
	code, message, ok := domain.Public(err)
	if !ok || code != "driver_create_failed" {
		t.Errorf("Create code = %q (ok=%v), want driver_create_failed", code, ok)
	}
	if strings.Contains(message, "10.0.0.7") {
		t.Errorf("Create leaked the internal cause in %q", message)
	}
	if !errors.Is(err, raw) {
		t.Error("Create dropped the cause, so logs cannot show why storage failed")
	}
}

func TestServiceKeepsClassifiedRepositoryError(t *testing.T) {
	conflict := domain.Conflict("driver_exists", "driver dupe already exists")
	svc := driver.NewService(stubRepo{err: conflict})

	_, err := svc.Create(context.Background(), driver.CreateInput{Name: "Ada"})
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
	if code, _, _ := domain.Public(err); code != "driver_exists" {
		t.Errorf("Create code = %q, want driver_exists", code)
	}
}

func TestServiceReportsCancelledRequestAsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := newService()
	_, err := svc.Create(ctx, driver.CreateInput{Name: "Ada"})
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
