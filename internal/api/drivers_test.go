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
