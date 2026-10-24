package vehicle_test

import (
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/vehicle"
)

func TestStatusString(t *testing.T) {
	tests := []struct {
		name   string
		status vehicle.Status
		want   string
	}{
		{"available", vehicle.StatusAvailable, "available"},
		{"on trip", vehicle.StatusOnTrip, "on_trip"},
		{"offline", vehicle.StatusOffline, "offline"},
		{"out of range", vehicle.Status(99), "unknown"},
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
	for _, want := range []vehicle.Status{
		vehicle.StatusAvailable,
		vehicle.StatusOnTrip,
		vehicle.StatusOffline,
	} {
		got, err := vehicle.ParseStatus(want.String())
		if err != nil {
			t.Fatalf("ParseStatus(%q): %v", want.String(), err)
		}
		if got != want {
			t.Errorf("ParseStatus(%q) = %d, want %d", want.String(), uint8(got), uint8(want))
		}
		if !got.Valid() {
			t.Errorf("ParseStatus(%q) returned an invalid status", want.String())
		}
	}
}

func TestParseStatusRejectsUnknownNames(t *testing.T) {
	for _, name := range []string{"", "available ", "Available", "flying", "on trip", "0"} {
		got, err := vehicle.ParseStatus(name)
		if err == nil {
			t.Fatalf("ParseStatus(%q) accepted an unknown status as %d", name, uint8(got))
		}
		if kind := domain.KindOf(err); kind != domain.KindInvalid {
			t.Errorf("ParseStatus(%q) kind = %s, want invalid", name, kind)
		}
		code, _, ok := domain.Public(err)
		if !ok || code != "vehicle_status_unknown" {
			t.Errorf("ParseStatus(%q) code = %q (ok=%v), want vehicle_status_unknown", name, code, ok)
		}
	}
}

func TestNewReturnsValidatedVehicle(t *testing.T) {
	got, err := vehicle.New("  B 1234 XY  ", "  van  ", "on_trip")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got.Name != "B 1234 XY" {
		t.Errorf("Name = %q, want %q", got.Name, "B 1234 XY")
	}
	if got.Type != "van" {
		t.Errorf("Type = %q, want %q", got.Type, "van")
	}
	if got.Status != vehicle.StatusOnTrip {
		t.Errorf("Status = %s, want on_trip", got.Status)
	}
	if got.ID != "" {
		t.Errorf("New assigned an id %q, want the service to assign it", got.ID)
	}
}

func TestNewRejectsBlankName(t *testing.T) {
	for _, name := range []string{"", "   ", "\t\n"} {
		got, err := vehicle.New(name, "van", "available")
		if err == nil {
			t.Fatalf("New(%q) accepted a blank name as %q", name, got.Name)
		}
		if kind := domain.KindOf(err); kind != domain.KindInvalid {
			t.Errorf("New(%q) kind = %s, want invalid", name, kind)
		}
		code, _, ok := domain.Public(err)
		if !ok || code != "vehicle_name_required" {
			t.Errorf("New(%q) code = %q (ok=%v), want vehicle_name_required", name, code, ok)
		}
	}
}

func TestNewRejectsUnknownStatus(t *testing.T) {
	got, err := vehicle.New("B 1", "van", "flying")
	if err == nil {
		t.Fatalf("New accepted an unknown status as %s", got.Status)
	}

	if kind := domain.KindOf(err); kind != domain.KindInvalid {
		t.Errorf("New kind = %s, want invalid", kind)
	}
	code, message, ok := domain.Public(err)
	if !ok || code != "vehicle_status_unknown" {
		t.Errorf("New code = %q (ok=%v), want vehicle_status_unknown", code, ok)
	}
	if message == "" {
		t.Error("New returned an empty message, want a clear error for the client")
	}
}

func TestNewProfileKeepsMissingValuesUnknownAndValidatesSuppliedValues(t *testing.T) {
	unknown, err := vehicle.NewProfile("Fleet van", "van", "available", "", "", "", nil, "", "", "", nil, "")
	if err != nil {
		t.Fatalf("NewProfile with unknown profile fields: %v", err)
	}
	if unknown.Registration != "" || unknown.Make != "" || unknown.Model != "" ||
		unknown.Year != nil || unknown.Color != "" || unknown.BodyType != "" ||
		unknown.FuelType != "" || unknown.Capacity != nil || unknown.MaintenanceHistory != "" {
		t.Errorf("missing profile values were changed: %+v", unknown)
	}

	year, capacity := 2022, 1250.5
	profile, err := vehicle.NewProfile(" Fleet van ", " van ", "available",
		" B 1234 XYZ ", " Example ", " Transit ", &year, " white ", " van ", " diesel ",
		&capacity, " 2025-01 oil service ")
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	if profile.Name != "Fleet van" || profile.Registration != "B 1234 XYZ" ||
		profile.Make != "Example" || profile.Model != "Transit" || profile.Color != "white" ||
		profile.BodyType != "van" || profile.FuelType != "diesel" ||
		profile.MaintenanceHistory != "2025-01 oil service" ||
		profile.Year == nil || *profile.Year != year || profile.Capacity == nil || *profile.Capacity != capacity {
		t.Errorf("NewProfile returned %+v with unnormalized or missing profile values", profile)
	}

	for _, invalid := range []struct {
		year     *int
		capacity *float64
	}{
		{year: ptrInt(1800)},
		{capacity: ptrFloat(0)},
		{capacity: ptrFloat(-1)},
	} {
		if _, err := vehicle.NewProfile("Fleet van", "van", "available", "", "", "", invalid.year, "", "", "", invalid.capacity, ""); err == nil {
			t.Errorf("NewProfile accepted invalid profile values: %+v", invalid)
		}
	}
}

func ptrInt(value int) *int           { return &value }
func ptrFloat(value float64) *float64 { return &value }
