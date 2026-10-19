package api

import (
	"net/http"
	"net/http/httptest"
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
	for _, key := range []string{"started_at", "finished_at", "duration_s"} {
		if value, exists := created[key]; !exists || value != nil {
			t.Errorf("%s = %v (present %t), want null before the trip starts", key, value, exists)
		}
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

// setTripStatus drives POST /api/trips/{id}/status.
func setTripStatus(t *testing.T, s *Server, tripID, body string) *httptest.ResponseRecorder {
	t.Helper()

	return postJSON(t, s, "/api/trips/"+tripID+"/status", body)
}

// TestTrips_AdvanceStatusThroughItsLife follows one trip from planned to
// completed and checks the list agrees, since a dashboard reads the list rather
// than the reply to the move.
func TestTrips_AdvanceStatusThroughItsLife(t *testing.T) {
	s := fleetServer(t)
	id := field(t, decodeBody(t, postJSON(t, s, "/api/trips", `{"origin":"Depot","destination":"Port"}`)), "id")

	for _, want := range []string{"in_progress", "completed"} {
		rec := setTripStatus(t, s, id, `{"status":"`+want+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("moving to %s: status = %d: %s", want, rec.Code, rec.Body)
		}
		updated := decodeBody(t, rec)
		if got := field(t, updated, "status"); got != want {
			t.Errorf("status = %q, want %q", got, want)
		}
		if updated["started_at"] == nil {
			t.Errorf("status %s has no recorded start time", want)
		}
		if want == "completed" {
			if updated["finished_at"] == nil || updated["duration_s"] == nil {
				t.Errorf("completed trip has no finish or duration: %v", updated)
			}
		}
	}

	list := listOf(t, decodeBody(t, getJSON(t, s, "/api/trips")), "trips")
	if len(list) != 1 {
		t.Fatalf("list returned %d trips, want 1", len(list))
	}
	if got := field(t, list[0].(map[string]any), "status"); got != "completed" {
		t.Errorf("listed status = %q, want completed", got)
	}
}

// TestTrips_RefuseAMoveBackwards keeps a finished trip finished: it is a record of
// what happened, not a state to rewind.
func TestTrips_RefuseAMoveBackwards(t *testing.T) {
	s := fleetServer(t)
	id := field(t, decodeBody(t, postJSON(t, s,
		"/api/trips", `{"origin":"A","destination":"B","status":"completed"}`)), "id")

	rec := setTripStatus(t, s, id, `{"status":"planned"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "trip_status_regressed" {
		t.Errorf("code = %q, want trip_status_regressed", code)
	}
}

// TestTrips_RejectAnUnknownStatusOnAMove covers a mistyped token and a missing
// one: a move has no default, because there is no "whatever comes next".
func TestTrips_RejectAnUnknownStatusOnAMove(t *testing.T) {
	s := fleetServer(t)
	id := field(t, decodeBody(t, postJSON(t, s, "/api/trips", `{"origin":"A","destination":"B"}`)), "id")

	for _, body := range []string{`{"status":"flying"}`, `{}`} {
		rec := setTripStatus(t, s, id, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400: %s", body, rec.Code, rec.Body)
		}
		if code := field(t, decodeBody(t, rec), "code"); code != "trip_status_unknown" {
			t.Errorf("body %s: code = %q, want trip_status_unknown", body, code)
		}
	}
}

func TestTrips_MovingAnUnknownTripIsNotFound(t *testing.T) {
	s := fleetServer(t)

	rec := setTripStatus(t, s, "nobody", `{"status":"in_progress"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "trip_not_found" {
		t.Errorf("code = %q, want trip_not_found", code)
	}
}
