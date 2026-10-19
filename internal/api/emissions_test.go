package api

import (
	"net/http"
	"testing"
)

func TestEmissions_EstimatesOneTrip(t *testing.T) {
	server := simServer(t)
	rec := getJSON(t, server, "/api/emissions?class=light_commercial&distance=12&speed=28")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/emissions = %d: %s", rec.Code, rec.Body)
	}

	body := decodeBody(t, rec)
	if got := field(t, body, "method"); got == "" {
		t.Error("response has no method disclaimer")
	}
	trip := object(t, body, "trip")
	if got := field(t, trip, "class"); got != "light_commercial" {
		t.Errorf("trip class = %q", got)
	}
	if got := number(t, trip, "distance_km"); got != 12 {
		t.Errorf("distance = %v, want 12 km", got)
	}
	if got := number(t, trip, "co2_g"); got <= 0 {
		t.Errorf("CO2 = %v, want a positive estimate", got)
	}
	if got := number(t, trip, "co2_per_km"); got <= 0 {
		t.Errorf("CO2 intensity = %v, want a positive estimate", got)
	}
	if run, exists := body["run"]; !exists || run != nil {
		t.Errorf("run = %v, want null when no comparison is active", run)
	}
}

func TestEmissions_RejectsUnknownVehicleClass(t *testing.T) {
	server := simServer(t)
	rec := getJSON(t, server, "/api/emissions?class=spaceship&distance=12&speed=28")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET invalid estimate = %d, want 400: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "emissions_class_unknown" {
		t.Errorf("error code = %q, want emissions_class_unknown", code)
	}
}
