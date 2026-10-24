package api

import (
	"net/http"
	"testing"
)

func TestDrivers_CreateThenList(t *testing.T) {
	s := fleetServer(t)
	rec := postJSON(t, s, "/api/drivers", `{"name":"Ada Lovelace","contact":"ada@example.com"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body)
	}
	created := decodeBody(t, rec)
	if got := field(t, created, "contact"); got != "ada@example.com" {
		t.Errorf("contact = %q", got)
	}
	if _, hasVehicle := created["vehicle_id"]; hasVehicle {
		t.Errorf("a new driver came with vehicle_id = %v", created["vehicle_id"])
	}
	for _, key := range []string{"contact_phone", "license_class", "license_expiry", "shift_start", "shift_end", "availability"} {
		if value, exists := created[key]; !exists || value != nil {
			t.Errorf("missing profile field %q = %v (exists=%v), want explicit unknown null", key, value, exists)
		}
	}

	rec = getJSON(t, s, "/api/drivers")
	list := listOf(t, decodeBody(t, rec), "drivers")
	if len(list) != 1 {
		t.Fatalf("list returned %d drivers, want 1", len(list))
	}
}

func TestDrivers_RejectABlankName(t *testing.T) {
	s := fleetServer(t)
	rec := postJSON(t, s, "/api/drivers", `{"name":"  "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "driver_name_required" {
		t.Errorf("code = %q, want driver_name_required", code)
	}
}

func TestDrivers_Answer503WithoutAService(t *testing.T) {
	s := testServer(t)
	rec := getJSON(t, s, "/api/drivers")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "fleet_unavailable" {
		t.Errorf("code = %q, want fleet_unavailable", code)
	}
}

func TestDrivers_CreateExposesProfile(t *testing.T) {
	s := fleetServer(t)
	rec := postJSON(t, s, "/api/drivers", `{"name":"Ada Lovelace","contact_phone":"+62 812-3456-7890","license_class":"B1","license_expiry":"2027-05-31","shift_start":"08:30","shift_end":"17:45","availability":"available"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body)
	}
	created := decodeBody(t, rec)
	for key, want := range map[string]string{
		"contact_phone": "+62 812-3456-7890", "license_class": "B1",
		"license_expiry": "2027-05-31", "shift_start": "08:30", "shift_end": "17:45",
		"availability": "available",
	} {
		if got := field(t, created, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	list := getJSON(t, s, "/api/drivers")
	item := listOf(t, decodeBody(t, list), "drivers")[0].(map[string]any)
	if item["contact_phone"] != "+62 812-3456-7890" || item["license_expiry"] != "2027-05-31" ||
		item["shift_start"] != "08:30" || item["shift_end"] != "17:45" {
		t.Errorf("list profile = %v", item)
	}
}

func TestDrivers_RejectInvalidProfileValues(t *testing.T) {
	for _, body := range []string{
		`{"name":"Ada","contact_phone":"call me"}`,
		`{"name":"Ada","license_expiry":"2027-02-30"}`,
		`{"name":"Ada","shift_start":"08:00"}`,
		`{"name":"Ada","shift_start":"8:00","shift_end":"17:00"}`,
		`{"name":"Ada","shift_start":"08:60","shift_end":"17:00"}`,
		`{"name":"Ada","shift_start":"08:00","shift_end":"24:00"}`,
		`{"name":"Ada","availability":"maybe"}`,
	} {
		s := fleetServer(t)
		rec := postJSON(t, s, "/api/drivers", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400: %s", body, rec.Code, rec.Body)
		}
	}
}
