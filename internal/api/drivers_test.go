package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestDrivers_AssignVehicle(t *testing.T) {
	s := fleetServer(t)
	rec := postJSON(t, s, "/api/drivers", `{"name":"Joko Widodo","contract_type":"Full-time"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body)
	}
	driverID := field(t, decodeBody(t, rec), "id")

	assignRec := postJSON(t, s, "/api/drivers/"+driverID+"/vehicle", `{"vehicle_id":"B-1000-JKT"}`)
	if assignRec.Code != http.StatusOK {
		t.Fatalf("assign status = %d: %s", assignRec.Code, assignRec.Body)
	}
	assigned := decodeBody(t, assignRec)
	if got := field(t, assigned, "vehicle_id"); got != "B-1000-JKT" {
		t.Errorf("vehicle_id = %q, want B-1000-JKT", got)
	}
	if got := field(t, assigned, "contract_type"); got != "Full-time" {
		t.Errorf("contract_type = %q, want Full-time", got)
	}
}

func TestDrivers_UpdateProfilePreservesVehicleAssignment(t *testing.T) {
	s := fleetServer(t)
	created := postJSON(t, s, "/api/drivers", `{"name":"FleetFlow Demo Driver 2","contract_type":"Contract"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body)
	}
	id := field(t, decodeBody(t, created), "id")
	if assigned := postJSON(t, s, "/api/drivers/"+id+"/vehicle", `{"vehicle_id":"vehicle-2"}`); assigned.Code != http.StatusOK {
		t.Fatalf("assign status = %d: %s", assigned.Code, assigned.Body)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/drivers/"+id, strings.NewReader(
		`{"name":"Demo Driver 2","contact":"demo-driver-2@example.invalid","contact_phone":"00000000002","license_class":"B1","license_expiry":"2030-12-31","availability":"available","contract_type":"Full-time"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", rec.Code, rec.Body)
	}
	updated := decodeBody(t, rec)
	if field(t, updated, "name") != "Demo Driver 2" ||
		field(t, updated, "contact_phone") != "00000000002" ||
		field(t, updated, "contract_type") != "Full-time" ||
		field(t, updated, "vehicle_id") != "vehicle-2" {
		t.Errorf("updated driver = %v, want profile fields and vehicle assignment", updated)
	}
}

func TestDrivers_UpdateRejectsUnknownContractType(t *testing.T) {
	s := fleetServer(t)
	created := postJSON(t, s, "/api/drivers", `{"name":"Demo Driver"}`)
	id := field(t, decodeBody(t, created), "id")
	req := httptest.NewRequest(http.MethodPut, "/api/drivers/"+id, strings.NewReader(
		`{"name":"Demo Driver","contract_type":"Permanent"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
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
