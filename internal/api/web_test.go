package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestConsoleIsPublic checks GET / serves the console page to a caller who
// presents no API key. The page is not an ingest endpoint: it is what a browser
// opens before any key exists, so requiring one would prevent users from
// reaching the console.
func TestConsoleIsPublic(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		if rec.Code == http.StatusServiceUnavailable && strings.Contains(rec.Body.String(), "console is not built") {
			return
		}
		t.Fatalf("GET / answered %d, want 200 or an explicit unbuilt-console response", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / answered Content-Type %q, want text/html", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, `id="root"`) {
		t.Error("GET / body has no React root: the root is served but is not the console")
	}
}

// TestDashboardDoesNotShadowTheAPI checks the catch-all page route leaves the
// API alone. ServeMux picks the most specific pattern, and this is the test that
// says so rather than assuming it.
func TestDashboardDoesNotShadowTheAPI(t *testing.T) {
	srv := testServer(t)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz answered %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("GET /healthz answered Content-Type %q, want application/json", ct)
	}
}
