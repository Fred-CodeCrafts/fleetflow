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

func (s stubRepo) Assign(context.Context, string, string) (driver.Driver, error) {
	return driver.Driver{}, s.err
}

func (s stubRepo) UpdateProfile(context.Context, string, driver.Driver) (driver.Driver, error) {
	return driver.Driver{}, s.err
}

func TestServiceUpdateProfilePreservesAssignedVehicle(t *testing.T) {
	repo := memory.New()
	svc := driver.NewService(repo)
	created, err := svc.Create(context.Background(), driver.CreateInput{
		Name: "Old Demo Name", ContractType: "Contract",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Assign(context.Background(), created.ID, "vehicle-2"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	got, err := svc.UpdateProfile(context.Background(), created.ID, driver.CreateInput{
		Name: "Demo Driver 2", Contact: "demo-driver-2@example.invalid",
		ContactPhone: "00000000002", LicenseClass: "B1",
		LicenseExpiry: "2030-12-31", ContractType: "Full-time",
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if got.Name != "Demo Driver 2" || got.ContractType != "Full-time" ||
		got.VehicleID != "vehicle-2" || got.ContactPhone != "00000000002" {
		t.Errorf("UpdateProfile returned %+v, want updated profile retaining assignment", got)
	}
}

func TestServiceUpdateProfileRejectsUnknownContractType(t *testing.T) {
	svc := newService()
	created, err := svc.Create(context.Background(), driver.CreateInput{Name: "Demo Driver"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.UpdateProfile(context.Background(), created.ID, driver.CreateInput{
		Name: "Demo Driver", ContractType: "Permanent",
	})
	if code, _, ok := domain.Public(err); !ok || code != "driver_contract_type_unknown" {
		t.Errorf("UpdateProfile error = %v, want contract type validation error", err)
	}
}

func TestServiceCreateRejectsUnknownContractType(t *testing.T) {
	svc := newService()
	_, err := svc.Create(context.Background(), driver.CreateInput{
		Name: "Demo Driver", ContractType: "Permanent",
	})
	if code, _, ok := domain.Public(err); !ok || code != "driver_contract_type_unknown" {
		t.Errorf("Create error = %v, want contract type validation error", err)
	}
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

func TestServiceAssignGivesTheDriverAVehicle(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	d, err := svc.Create(ctx, driver.CreateInput{Name: "Ada Lovelace"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Assign(ctx, d.ID, "v1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if got.VehicleID != "v1" {
		t.Errorf("Assign returned VehicleID %q, want v1", got.VehicleID)
	}

	// The assignment must be visible through List, not only in the result.
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].VehicleID != "v1" {
		t.Errorf("List returned %+v, want the assigned driver", list)
	}
}

// TestServiceAssignTrimsTheVehicleID covers the wire: a client that sends
// "  " means "no vehicle", and the service must not store whitespace as if it
// named a vehicle.
func TestServiceAssignTrimsTheVehicleID(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	d, err := svc.Create(ctx, driver.CreateInput{Name: "Ada Lovelace"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Assign(ctx, d.ID, " v1 "); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	got, err := svc.Assign(ctx, d.ID, "   ")
	if err != nil {
		t.Fatalf("Assign unassign: %v", err)
	}
	if got.VehicleID != "" {
		t.Errorf("Assign stored VehicleID %q, want empty", got.VehicleID)
	}
}

func TestServiceAssignRejectsABlankDriverID(t *testing.T) {
	svc := newService()

	_, err := svc.Assign(context.Background(), "   ", "v1")
	if err == nil {
		t.Fatal("Assign accepted a blank driver id")
	}
	if kind := domain.KindOf(err); kind != domain.KindInvalid {
		t.Errorf("Assign kind = %s, want invalid", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "driver_id_required" {
		t.Errorf("Assign code = %q (ok=%v), want driver_id_required", code, ok)
	}
}

// TestServiceAssignKeepsTheRepositoryOutcome checks the classification path for
// Assign specifically: the code in the body has to name the operation that
// failed, not reuse Create's.
func TestServiceAssignKeepsTheRepositoryOutcome(t *testing.T) {
	svc := driver.NewService(stubRepo{err: errors.New("pq: connection refused")})

	_, err := svc.Assign(context.Background(), "a1", "v1")
	if err == nil {
		t.Fatal("Assign succeeded although the repository failed")
	}

	if code, _, ok := domain.Public(err); !ok || code != "driver_assign_failed" {
		t.Errorf("Assign code = %q (ok=%v), want driver_assign_failed", code, ok)
	}

	taken := domain.Conflict("driver_vehicle_taken", "vehicle v1 already belongs to driver a2")
	_, err = driver.NewService(stubRepo{err: taken}).Assign(context.Background(), "a1", "v1")
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Assign kind = %s, want conflict", kind)
	}
	if code, _, _ := domain.Public(err); code != "driver_vehicle_taken" {
		t.Errorf("Assign code = %q, want driver_vehicle_taken", code)
	}
}

func TestNewProfileKeepsMissingValuesUnknownAndValidatesSuppliedValues(t *testing.T) {
	unknown, err := driver.NewProfile("Ada Lovelace", "", "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("NewProfile with unknown profile fields: %v", err)
	}
	if unknown.ContactPhone != "" || unknown.LicenseClass != "" || unknown.LicenseExpiry != "" ||
		unknown.ShiftStart != "" || unknown.ShiftEnd != "" || unknown.Availability != "" {
		t.Errorf("missing profile values were changed: %+v", unknown)
	}

	profile, err := driver.NewProfile(" Ada Lovelace ", "", "+62 812-3456-7890", " B1 ", "2027-05-31", " 08:30 ", "17:45", "available")
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	if profile.Name != "Ada Lovelace" || profile.ContactPhone != "+62 812-3456-7890" ||
		profile.LicenseClass != "B1" || profile.LicenseExpiry != "2027-05-31" ||
		profile.ShiftStart != "08:30" || profile.ShiftEnd != "17:45" || profile.Availability != "available" {
		t.Errorf("NewProfile returned %+v with unnormalized or missing values", profile)
	}

	for _, input := range []struct {
		phone, expiry, shiftStart, shiftEnd, availability string
	}{
		{phone: "call me"},
		{phone: "++628123456789"},
		{phone: "(81234567"},
		{expiry: "2027-02-30"},
		{shiftStart: "08:00"},
		{shiftEnd: "17:00"},
		{shiftStart: "8:00", shiftEnd: "17:00"},
		{shiftStart: "24:00", shiftEnd: "17:00"},
		{shiftStart: "08:60", shiftEnd: "17:00"},
		{availability: "maybe"},
	} {
		if _, err := driver.NewProfile("Ada", "", input.phone, "", input.expiry, input.shiftStart, input.shiftEnd, input.availability); err == nil {
			t.Errorf("NewProfile accepted invalid values: %+v", input)
		}
	}

	if _, err := driver.NewProfile("Ada", "", "", "", "", "00:00", "23:59", ""); err != nil {
		t.Errorf("NewProfile rejected valid boundary clock times: %v", err)
	}
}
