package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fleetflow/internal/api"
	"fleetflow/internal/trip"
	tripmemory "fleetflow/internal/trip/memory"
)

func TestTripEndpoints(t *testing.T) {
	repo := tripmemory.New()
	svc := trip.NewService(repo)
	srv := api.NewServer(api.WithTrips(svc))

	// 1. List initially empty
	req := httptest.NewRequest(http.MethodGet, "/api/trips", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/trips returned %d, want 200", rec.Code)
	}

	var listRes struct {
		Trips []struct {
			ID     string `json:"id"`
			Origin string `json:"origin"`
			Status string `json:"status"`
		} `json:"trips"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listRes); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listRes.Trips) != 0 {
		t.Fatalf("expected 0 trips initially, got %d", len(listRes.Trips))
	}

	// 2. Create planned trip
	createBody := map[string]string{
		"origin":      "Tanjung Priok Port Terminal",
		"destination": "Kelapa Gading Commercial District",
		"vehicle_id":  "B-9012-JKT",
		"status":      "planned",
	}
	bodyBytes, _ := json.Marshal(createBody)
	req = httptest.NewRequest(http.MethodPost, "/api/trips", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/trips returned %d: %s", rec.Code, rec.Body.String())
	}

	var created struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		VehicleID string `json:"vehicle_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.ID == "" || created.Status != "planned" || created.VehicleID != "B-9012-JKT" {
		t.Fatalf("unexpected created trip: %+v", created)
	}

	// 3. Advance to in_progress
	patchBody := map[string]string{"status": "in_progress"}
	pBytes, _ := json.Marshal(patchBody)
	req = httptest.NewRequest(http.MethodPatch, "/api/trips/"+created.ID+"/status", bytes.NewReader(pBytes))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /api/trips/{id}/status returned %d: %s", rec.Code, rec.Body.String())
	}

	var updated struct {
		ID        string  `json:"id"`
		Status    string  `json:"status"`
		StartedAt *string `json:"started_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal patch response: %v", err)
	}
	if updated.Status != "in_progress" || updated.StartedAt == nil {
		t.Fatalf("expected in_progress status with started_at, got %+v", updated)
	}
}
