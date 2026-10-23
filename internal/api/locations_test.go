package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocationsAcceptsGPSPositionAndListsIt(t *testing.T) {
	s := testServer(t)
	rec := postJSON(t, s, "/api/locations",
		`{"vehicle_id":"van-17","lat":-6.2088,"lon":106.8456,"speed_mps":8.5}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, want 202: %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/locations", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", rec.Code, rec.Body)
	}
	positions, ok := decodeBody(t, rec)["positions"].([]any)
	if !ok || len(positions) != 1 {
		t.Fatalf("positions = %v", decodeBody(t, rec)["positions"])
	}
	position, ok := positions[0].(map[string]any)
	if !ok || position["vehicle_id"] != "van-17" || position["lat"] != -6.2088 || position["lon"] != 106.8456 {
		t.Fatalf("position = %v", positions[0])
	}
}

func TestLocationsRejectsInvalidCoordinates(t *testing.T) {
	s := testServer(t)
	rec := postJSON(t, s, "/api/locations", `{"vehicle_id":"van-17","lat":-91,"lon":106.8}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/locations", nil))
	positions, ok := decodeBody(t, rec)["positions"].([]any)
	if !ok || len(positions) != 0 {
		t.Fatalf("positions after rejected input = %v", decodeBody(t, rec)["positions"])
	}
}
