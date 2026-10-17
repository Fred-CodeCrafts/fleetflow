package api

import (
	"net/http"
	"testing"
)

// createDriver creates a driver through the API and returns its id.
func createDriver(t *testing.T, s *Server, name string) string {
	t.Helper()

	rec := postJSON(t, s, "/api/drivers", `{"name":"`+name+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating driver %q: status %d: %s", name, rec.Code, rec.Body)
	}
	return field(t, decodeBody(t, rec), "id")
}

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

func TestAssignDriver_PutsADriverOnAVehicleAndTakesItOff(t *testing.T) {
	s := fleetServer(t)
	vehicleID := createVehicle(t, s, "van-1")
	driverID := createDriver(t, s, "Ada Lovelace")

	rec := postJSON(t, s, "/api/vehicles/"+vehicleID+"/driver", `{"driver_id":"`+driverID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("assign status = %d, want 200: %s", rec.Code, rec.Body)
	}
	body := decodeBody(t, rec)
	if got := field(t, body, "vehicle_id"); got != vehicleID {
		t.Errorf("vehicle_id = %q, want %q", got, vehicleID)
	}
	assigned, ok := body["driver"].(map[string]any)
	if !ok {
		t.Fatalf("driver = %v, want the assigned driver", body["driver"])
	}
	if got := field(t, assigned, "id"); got != driverID {
		t.Errorf("driver id = %q, want %q", got, driverID)
	}

	// A blank driver_id takes whoever holds the vehicle off it.
	rec = postJSON(t, s, "/api/vehicles/"+vehicleID+"/driver", `{"driver_id":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unassign status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := decodeBody(t, rec)["driver"]; got != nil {
		t.Errorf("driver = %v, want null after unassigning", got)
	}

	// And the assignment is really gone from the driver, not just from the reply.
	rec = getJSON(t, s, "/api/drivers")
	list := listOf(t, decodeBody(t, rec), "drivers")
	if _, hasVehicle := list[0].(map[string]any)["vehicle_id"]; hasVehicle {
		t.Errorf("driver still holds a vehicle: %v", list[0])
	}
}

func TestAssignDriver_RefusesAVehicleAnotherDriverHolds(t *testing.T) {
	s := fleetServer(t)
	vehicleID := createVehicle(t, s, "van-1")
	first := createDriver(t, s, "Ada")
	second := createDriver(t, s, "Grace")

	rec := postJSON(t, s, "/api/vehicles/"+vehicleID+"/driver", `{"driver_id":"`+first+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("first assign status = %d: %s", rec.Code, rec.Body)
	}

	rec = postJSON(t, s, "/api/vehicles/"+vehicleID+"/driver", `{"driver_id":"`+second+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second assign status = %d, want 409: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "driver_vehicle_taken" {
		t.Errorf("code = %q, want driver_vehicle_taken", code)
	}
}

func TestAssignDriver_RefusesAnUnknownDriver(t *testing.T) {
	s := fleetServer(t)
	vehicleID := createVehicle(t, s, "van-1")

	rec := postJSON(t, s, "/api/vehicles/"+vehicleID+"/driver", `{"driver_id":"nobody"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "driver_not_found" {
		t.Errorf("code = %q, want driver_not_found", code)
	}
}

// TestAssignDriver_UnassigningAnEmptyVehicleSucceeds keeps the endpoint
// idempotent: a client that clears a vehicle twice is not making a mistake.
func TestAssignDriver_UnassigningAnEmptyVehicleSucceeds(t *testing.T) {
	s := fleetServer(t)
	vehicleID := createVehicle(t, s, "van-1")

	rec := postJSON(t, s, "/api/vehicles/"+vehicleID+"/driver", `{"driver_id":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := decodeBody(t, rec)["driver"]; got != nil {
		t.Errorf("driver = %v, want null", got)
	}
}
