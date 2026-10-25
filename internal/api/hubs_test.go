package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The map has to know every place a scenario can start or end. It used to keep
// its own copy of the hub list, which fell behind and left Soekarno-Hatta Cargo
// Terminal unmarked — a vehicle appearing to start in empty space. Serving the
// generator's catalogue is what stops that happening again.
func TestHubsAreServedFromTheScenarioCatalogue(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/hubs", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/hubs answered %d, want 200", rec.Code)
	}
	var body struct {
		Hubs []struct {
			Name      string  `json:"name"`
			Code      string  `json:"code"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Area      string  `json:"area"`
		} `json:"hubs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode hubs: %v", err)
	}
	if len(body.Hubs) == 0 {
		t.Fatal("no hubs were served, so the map has nothing to draw")
	}

	// These are the fields the map needs to place a marker, so a hub without
	// them is worse than no hub: it would be drawn at the wrong place.
	airport := false
	for _, hub := range body.Hubs {
		if hub.Name == "" || hub.Code == "" || hub.Area == "" {
			t.Fatalf("hub %#v is missing a name, code or area", hub)
		}
		if hub.Latitude == 0 || hub.Longitude == 0 {
			t.Fatalf("hub %q has no position", hub.Code)
		}
		if hub.Name == "Soekarno-Hatta Cargo Terminal" {
			airport = true
		}
	}
	if !airport {
		t.Fatal("the airport cargo terminal is missing from the served hubs, so a scenario can start there unmarked")
	}
}
