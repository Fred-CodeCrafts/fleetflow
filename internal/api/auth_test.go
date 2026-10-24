package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postJSONAs(t *testing.T, s *Server, path, body, header string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestLocation_RejectsMissingOrInvalidAPIKey(t *testing.T) {
	s := testServer(t)
	body := `{"vehicle_id":"unit-1","lat":-6.2,"lon":106.8}`
	for _, header := range []string{"", "Basic " + testAPIKey, "Bearer", "Bearer unknown"} {
		rec := postJSONAs(t, s, "/api/locations", body, header)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q status = %d, want 401: %s", header, rec.Code, rec.Body)
		}
	}
}

func TestLocation_AcceptsAStoredAPIKey(t *testing.T) {
	s := testServer(t)
	rec := postJSONAs(t, s, "/api/locations",
		`{"vehicle_id":"unit-1","lat":-6.2,"lon":106.8,"speed_mps":7}`,
		"Bearer "+testAPIKey)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("location status = %d, want 202: %s", rec.Code, rec.Body)
	}

	list := httptest.NewRecorder()
	s.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/locations", nil))
	positions, ok := decodeBody(t, list)["positions"].([]any)
	if !ok || len(positions) != 1 {
		t.Fatalf("positions = %v", decodeBody(t, list)["positions"])
	}
}

func TestLocation_ReportsAMissingKeyStore(t *testing.T) {
	s := NewServer(WithRouter(testRouter{}),
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	rec := postJSONAs(t, s, "/api/locations",
		`{"vehicle_id":"unit-1","lat":-6.2,"lon":106.8}`, "Bearer "+testAPIKey)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "auth_unavailable" {
		t.Errorf("code = %q, want auth_unavailable", code)
	}
}
