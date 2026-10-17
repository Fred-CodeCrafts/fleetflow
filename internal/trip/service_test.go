package trip_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/trip"
	"fleetflow/internal/trip/memory"
)

// newService returns a Service backed by a fresh in-memory repository, which is
// how DESIGN 3.7 asks services to be unit-tested.
func newService() *trip.Service { return trip.NewService(memory.New()) }

func TestServiceCreateStoresTrip(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	got, err := svc.Create(ctx, trip.CreateInput{
		Origin:      "Depot A",
		Destination: "Depot B",
		VehicleID:   "v1",
		Status:      "planned",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == "" {
		t.Error("Create returned an empty id, want a generated one")
	}
	if got.Origin != "Depot A" || got.Destination != "Depot B" ||
		got.VehicleID != "v1" || got.Status != trip.StatusPlanned {
		t.Errorf("Create returned %+v, want the values it was given", got)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0] != got {
		t.Errorf("List returned %+v, want the created trip %+v", list, got)
	}
}

func TestServiceCreateRejectsInvalidInputAndSavesNothing(t *testing.T) {
	tests := []struct {
		name     string
		in       trip.CreateInput
		wantCode string
	}{
		{"empty origin", trip.CreateInput{Destination: "B", Status: "planned"}, "trip_origin_required"},
		{"blank origin", trip.CreateInput{Origin: "   ", Destination: "B", Status: "planned"}, "trip_origin_required"},
		{"empty destination", trip.CreateInput{Origin: "A", Status: "planned"}, "trip_destination_required"},
		{"blank destination", trip.CreateInput{Origin: "A", Destination: "   ", Status: "planned"}, "trip_destination_required"},
		{"unknown status", trip.CreateInput{Origin: "A", Destination: "B", Status: "flying"}, "trip_status_unknown"},
		{"empty status", trip.CreateInput{Origin: "A", Destination: "B"}, "trip_status_unknown"},
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

			list, err := svc.List(ctx)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(list) != 0 {
				t.Errorf("invalid input saved %d trip(s)", len(list))
			}
		})
	}
}

func TestServiceListReturnsCreatedTripsInCreationOrder(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	origins := []string{"first", "second", "third"}
	for _, origin := range origins {
		in := trip.CreateInput{Origin: origin, Destination: "B", Status: "planned"}
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("Create(%q): %v", origin, err)
		}
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(origins) {
		t.Fatalf("List returned %d trips, want %d", len(list), len(origins))
	}
	for i, origin := range origins {
		if list[i].Origin != origin {
			t.Errorf("list[%d].Origin = %q, want %q", i, list[i].Origin, origin)
		}
	}
}

func TestServiceIDsAreUnique(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	const count = 200
	seen := make(map[string]bool, count)
	for i := 0; i < count; i++ {
		in := trip.CreateInput{Origin: fmt.Sprintf("A-%d", i), Destination: "B", Status: "planned"}
		got, err := svc.Create(ctx, in)
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
		t.Errorf("List returned %d trips, want %d", len(list), count)
	}
}

// stubRepo is a Repository that always fails, to check how the service
// classifies a storage failure without needing a database.
type stubRepo struct{ err error }

func (s stubRepo) Create(context.Context, trip.Trip) (trip.Trip, error) {
	return trip.Trip{}, s.err
}

func (s stubRepo) List(context.Context) ([]trip.Trip, error) {
	return nil, s.err
}

func (s stubRepo) Assign(context.Context, string, string) (trip.Trip, error) {
	return trip.Trip{}, s.err
}

func TestServiceHidesRawStorageError(t *testing.T) {
	raw := errors.New("pq: dial tcp 10.0.0.7:5432: connection refused")
	svc := trip.NewService(stubRepo{err: raw})

	in := trip.CreateInput{Origin: "A", Destination: "B", Status: "planned"}
	_, err := svc.Create(context.Background(), in)
	if err == nil {
		t.Fatal("Create succeeded although the repository failed")
	}
	if kind := domain.KindOf(err); kind != domain.KindInternal {
		t.Errorf("Create kind = %s, want internal", kind)
	}
	code, message, ok := domain.Public(err)
	if !ok || code != "trip_create_failed" {
		t.Errorf("Create code = %q (ok=%v), want trip_create_failed", code, ok)
	}
	if strings.Contains(message, "10.0.0.7") {
		t.Errorf("Create leaked the internal cause in %q", message)
	}
	if !errors.Is(err, raw) {
		t.Error("Create dropped the cause, so logs cannot show why storage failed")
	}
}

func TestServiceKeepsClassifiedRepositoryError(t *testing.T) {
	conflict := domain.Conflict("trip_exists", "trip dupe already exists")
	svc := trip.NewService(stubRepo{err: conflict})

	in := trip.CreateInput{Origin: "A", Destination: "B", Status: "planned"}
	_, err := svc.Create(context.Background(), in)
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
	if code, _, _ := domain.Public(err); code != "trip_exists" {
		t.Errorf("Create code = %q, want trip_exists", code)
	}
}

func TestServiceReportsCancelledRequestAsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := newService()
	in := trip.CreateInput{Origin: "A", Destination: "B", Status: "planned"}
	_, err := svc.Create(ctx, in)
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

func TestStatusString(t *testing.T) {
	tests := []struct {
		name   string
		status trip.Status
		want   string
	}{
		{"planned", trip.StatusPlanned, "planned"},
		{"in progress", trip.StatusInProgress, "in_progress"},
		{"completed", trip.StatusCompleted, "completed"},
		{"out of range", trip.Status(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("Status(%d).String() = %q, want %q", uint8(tt.status), got, tt.want)
			}
		})
	}
}

func TestParseStatusRoundTrip(t *testing.T) {
	want := []trip.Status{trip.StatusPlanned, trip.StatusInProgress, trip.StatusCompleted}
	for _, s := range want {
		got, err := trip.ParseStatus(s.String())
		if err != nil {
			t.Fatalf("ParseStatus(%q): %v", s.String(), err)
		}
		if got != s {
			t.Errorf("ParseStatus(%q) = %d, want %d", s.String(), uint8(got), uint8(s))
		}
		if !got.Valid() {
			t.Errorf("ParseStatus(%q) returned an invalid status", s.String())
		}
	}
}

func TestParseStatusRejectsUnknownNames(t *testing.T) {
	for _, name := range []string{"", "planned ", "Planned", "flying", "in progress", "0"} {
		got, err := trip.ParseStatus(name)
		if err == nil {
			t.Fatalf("ParseStatus(%q) accepted an unknown status as %d", name, uint8(got))
		}
		if kind := domain.KindOf(err); kind != domain.KindInvalid {
			t.Errorf("ParseStatus(%q) kind = %s, want invalid", name, kind)
		}
		code, _, ok := domain.Public(err)
		if !ok || code != "trip_status_unknown" {
			t.Errorf("ParseStatus(%q) code = %q (ok=%v), want trip_status_unknown", name, code, ok)
		}
	}
}

func TestServiceAssignPutsTheTripOnAVehicle(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	tr, err := svc.Create(ctx, trip.CreateInput{Origin: "A", Destination: "B", Status: "planned"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Assign(ctx, tr.ID, "v1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if got.VehicleID != "v1" {
		t.Errorf("Assign returned VehicleID %q, want v1", got.VehicleID)
	}

	// The assignment has to be visible through List, not only in the result.
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].VehicleID != "v1" {
		t.Errorf("List returned %+v, want the trip on v1", list)
	}
}

func TestServiceAssignRejectsABlankTripID(t *testing.T) {
	svc := newService()

	_, err := svc.Assign(context.Background(), "  ", "v1")
	if err == nil {
		t.Fatal("Assign accepted a blank trip id")
	}
	if kind := domain.KindOf(err); kind != domain.KindInvalid {
		t.Errorf("Assign kind = %s, want invalid", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "trip_id_required" {
		t.Errorf("Assign code = %q (ok=%v), want trip_id_required", code, ok)
	}
}

// TestServiceAssignKeepsTheRepositoryOutcome checks that a refusal from the store
// reaches the client as the store's own conflict, and that a storage failure
// names the operation that failed.
func TestServiceAssignKeepsTheRepositoryOutcome(t *testing.T) {
	svc := trip.NewService(stubRepo{err: errors.New("pq: connection refused")})

	_, err := svc.Assign(context.Background(), "t1", "v1")
	if err == nil {
		t.Fatal("Assign succeeded although the repository failed")
	}
	if code, _, ok := domain.Public(err); !ok || code != "trip_assign_failed" {
		t.Errorf("Assign code = %q (ok=%v), want trip_assign_failed", code, ok)
	}

	busy := domain.Conflict("trip_vehicle_busy", "vehicle v1 is already on trip t2")
	_, err = trip.NewService(stubRepo{err: busy}).Assign(context.Background(), "t1", "v1")
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Assign kind = %s, want conflict", kind)
	}
	if code, _, _ := domain.Public(err); code != "trip_vehicle_busy" {
		t.Errorf("Assign code = %q, want trip_vehicle_busy", code)
	}
}
