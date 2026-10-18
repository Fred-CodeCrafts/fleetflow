package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAssetsAreEmbedded fails if the embed directive ever stops naming a file
// the table serves. That is the failure mode of a build that runs and serves a
// blank dashboard, so it is worth a test rather than a comment.
func TestAssetsAreEmbedded(t *testing.T) {
	for _, a := range served {
		body, err := content.ReadFile(a.file)
		if err != nil {
			t.Errorf("asset %s is served at %s but is not embedded: %v", a.file, a.path, err)
			continue
		}
		if len(body) == 0 {
			t.Errorf("asset %s is embedded but empty", a.file)
		}
	}
}

// TestHandlerServesPageAtRoot checks the one URL a person and the browser both
// ask for: the root must be the page, as HTML.
func TestHandlerServesPageAtRoot(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / answered %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / answered Content-Type %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "<canvas") {
		t.Error("GET / body has no canvas: the page is served but is not the dashboard")
	}
}

// TestHandlerServesAssets checks every path in the table is served, with the
// type it declares, because a page that loads is a page whose assets resolved.
func TestHandlerServesAssets(t *testing.T) {
	handler := Handler()

	for _, a := range served {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, a.path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s answered %d, want 200", a.path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); ct != a.ctype {
			t.Errorf("GET %s answered Content-Type %q, want %q", a.path, ct, a.ctype)
		}
	}
}

// TestUnknownPathIsNotFound pins the 404, so a route that quietly serves the page
// for every path cannot pass unnoticed.
func TestUnknownPathIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/not-a-real-asset.js", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /not-a-real-asset.js answered %d, want 404", rec.Code)
	}
}
