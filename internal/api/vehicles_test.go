package api

import (
	"net/http"
	"testing"
)

func createVehicle(t *testing.T, s *Server, name string) string {
	t.Helper()
	rec := postJSON(t, s, "/api/vehicles", `{"name":"`+name+`","status":"available"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating vehicle %q: status %d: %s", name, rec.Code, rec.Body)
	}
	return field(t, decodeBody(t, rec), "id")
}

func TestVehicles_CreateThenList(t *testing.T) {
	s := fleetServer(t)
	rec := postJSON(t, s, "/api/vehicles", `{"name":"van-1","type":"van","status":"available"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body)
	}
	created := decodeBody(t, rec)
	id := field(t, created, "id")
	if id == "" {
		t.Fatal("create returned no id")
	}
	if got := field(t, created, "status"); got != "available" {
		t.Errorf("status = %q, want available", got)
	}
	for _, key := range []string{"registration", "make", "model", "year", "color", "body_type", "fuel_type", "capacity", "maintenance_history"} {
		if value, exists := created[key]; !exists || value != nil {
			t.Errorf("missing profile field %q = %v (exists=%v), want explicit unknown null", key, value, exists)
		}
	}
	if loc := rec.Header().Get("Location"); loc != "/api/vehicles/"+id {
		t.Errorf("Location = %q, want /api/vehicles/%s", loc, id)
	}

	rec = getJSON(t, s, "/api/vehicles")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", rec.Code, rec.Body)
	}
	list := listOf(t, decodeBody(t, rec), "vehicles")
	if len(list) != 1 {
		t.Fatalf("list returned %d vehicles, want 1", len(list))
	}
	first := list[0].(map[string]any)
	if field(t, first, "id") != id || field(t, first, "name") != "van-1" {
		t.Errorf("list returned %v, want the created vehicle", first)
	}
}

func TestVehicles_RejectBadInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{"blank name", `{"name":"   ","status":"available"}`, "vehicle_name_required"},
		{"unknown status", `{"name":"van-1","status":"flying"}`, "vehicle_status_unknown"},
		{"missing status", `{"name":"van-1"}`, "vehicle_status_unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := fleetServer(t)
			rec := postJSON(t, s, "/api/vehicles", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
			if code := field(t, decodeBody(t, rec), "code"); code != tt.code {
				t.Errorf("code = %q, want %q", code, tt.code)
			}
		})
	}
}

func TestVehicles_FilterByStatus(t *testing.T) {
	s := fleetServer(t)
	postJSON(t, s, "/api/vehicles", `{"name":"one","status":"available"}`)
	postJSON(t, s, "/api/vehicles", `{"name":"two","status":"offline"}`)

	rec := getJSON(t, s, "/api/vehicles?status=offline")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	list := listOf(t, decodeBody(t, rec), "vehicles")
	if len(list) != 1 {
		t.Fatalf("list returned %d vehicles, want 1", len(list))
	}
	if name := field(t, list[0].(map[string]any), "name"); name != "two" {
		t.Errorf("list returned %q, want two", name)
	}

	rec = getJSON(t, s, "/api/vehicles?status=flying")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unknown filter: %s", rec.Code, rec.Body)
	}
}

func TestVehicles_Answer503WithoutAService(t *testing.T) {
	s := testServer(t)
	rec := getJSON(t, s, "/api/vehicles")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "fleet_unavailable" {
		t.Errorf("code = %q, want fleet_unavailable", code)
	}
}

func TestVehicles_CreateExposesProfile(t *testing.T) {
	s := fleetServer(t)
	rec := postJSON(t, s, "/api/vehicles", `{"name":"Fleet van","type":"van","status":"available","registration":"B 1234 XYZ","make":"Example","model":"Transit","year":2022,"color":"white","body_type":"van","fuel_type":"diesel","capacity":1250.5,"maintenance_history":"2025-01 oil service"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body)
	}
	created := decodeBody(t, rec)
	for key, want := range map[string]string{
		"registration": "B 1234 XYZ", "make": "Example", "model": "Transit",
		"color": "white", "body_type": "van", "fuel_type": "diesel",
		"maintenance_history": "2025-01 oil service",
	} {
		if got := field(t, created, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if created["year"] != float64(2022) || created["capacity"] != 1250.5 {
		t.Errorf("year/capacity = %v/%v, want 2022/1250.5", created["year"], created["capacity"])
	}

	list := getJSON(t, s, "/api/vehicles")
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", list.Code, list.Body)
	}
	item := listOf(t, decodeBody(t, list), "vehicles")[0].(map[string]any)
	if item["registration"] != "B 1234 XYZ" || item["year"] != float64(2022) {
		t.Errorf("list profile = %v", item)
	}
}

func TestVehicles_RejectInvalidProfileValues(t *testing.T) {
	for _, body := range []string{
		`{"name":"van","status":"available","year":1800}`,
		`{"name":"van","status":"available","capacity":0}`,
	} {
		s := fleetServer(t)
		rec := postJSON(t, s, "/api/vehicles", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400: %s", body, rec.Code, rec.Body)
		}
	}
}
