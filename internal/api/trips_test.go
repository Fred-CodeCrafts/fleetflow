package api

import (
	"net/http"
	"testing"
)

func TestTrips_CreateDefaultsToPlanned(t *testing.T) {
	s := fleetServer(t)

	rec := postJSON(t, s, "/api/trips", `{"origin":"Depot","destination":"Port"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
	created := decodeBody(t, rec)
	if got := field(t, created, "status"); got != "planned" {
		t.Errorf("status = %q, want planned", got)
	}
	if _, hasVehicle := created["vehicle_id"]; hasVehicle {
		t.Errorf("a trip created with no vehicle has vehicle_id = %v", created["vehicle_id"])
	}
	if loc := rec.Header().Get("Location"); loc != "/api/trips/"+field(t, created, "id") {
		t.Errorf("Location = %q, want the created trip", loc)
	}

	rec = getJSON(t, s, "/api/trips")
	if list := listOf(t, decodeBody(t, rec), "trips"); len(list) != 1 {
		t.Fatalf("list returned %d trips, want 1", len(list))
	}
}

func TestTrips_CreateWithAVehicle(t *testing.T) {
	s := fleetServer(t)
	vehicleID := createVehicle(t, s, "van-1")

	rec := postJSON(t, s, "/api/trips", `{"origin":"Depot","destination":"Port","vehicle_id":"`+vehicleID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
	if got := field(t, decodeBody(t, rec), "vehicle_id"); got != vehicleID {
		t.Errorf("vehicle_id = %q, want %q", got, vehicleID)
	}
}

// TestTrips_AcceptAnExplicitStatus keeps the create honest about what it
// defaults: a client that does name a status gets the one it named.
func TestTrips_AcceptAnExplicitStatus(t *testing.T) {
	s := fleetServer(t)

	rec := postJSON(t, s, "/api/trips", `{"origin":"A","destination":"B","status":"in_progress"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
	if got := field(t, decodeBody(t, rec), "status"); got != "in_progress" {
		t.Errorf("status = %q, want in_progress", got)
	}
}

func TestTrips_RejectBadInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{"blank origin", `{"origin":"","destination":"Port"}`, "trip_origin_required"},
		{"blank destination", `{"origin":"Depot","destination":"  "}`, "trip_destination_required"},
		{"unknown status", `{"origin":"A","destination":"B","status":"flying"}`, "trip_status_unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := fleetServer(t)

			rec := postJSON(t, s, "/api/trips", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
			if code := field(t, decodeBody(t, rec), "code"); code != tt.code {
				t.Errorf("code = %q, want %q", code, tt.code)
			}
		})
	}
}

func TestTrips_Answer503WithoutAService(t *testing.T) {
	s := testServer(t)

	rec := getJSON(t, s, "/api/trips")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "fleet_unavailable" {
		t.Errorf("code = %q, want fleet_unavailable", code)
	}
}
