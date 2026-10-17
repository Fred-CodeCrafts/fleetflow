package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"fleetflow/internal/vehicle"
	vehiclememory "fleetflow/internal/vehicle/memory"
)

// fleetServer builds a server whose record endpoints are backed by the in-memory
// repositories. That is how DESIGN 3.7 asks the layers above storage to be
// tested: the transport and the rules, without a database. Each record type is
// added as its endpoints appear.
func fleetServer(t *testing.T) *Server {
	t.Helper()

	return testServer(t, WithVehicles(vehicle.NewService(vehiclememory.New())))
}

// getJSON sends a GET to path and returns the recorder the server wrote to.
func getJSON(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// field reads a required string field from a decoded body.
func field(t *testing.T, body map[string]any, key string) string {
	t.Helper()

	got, ok := body[key].(string)
	if !ok {
		t.Fatalf("body has no string %q: %v", key, body)
	}
	return got
}

// listOf reads an array field from a decoded body.
func listOf(t *testing.T, body map[string]any, key string) []any {
	t.Helper()

	got, ok := body[key].([]any)
	if !ok {
		t.Fatalf("body has no array %q: %v", key, body)
	}
	return got
}
