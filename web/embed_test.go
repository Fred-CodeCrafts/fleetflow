package web

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestBuiltConsoleAssetsAreEmbedded(t *testing.T) {
	if !consoleBuilt(t) {
		t.Skip("console assets are not built; run npm run build from console/")
	}

	found := false
	err := fs.WalkDir(content, "dist", func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		found = true
		body, err := content.ReadFile(file)
		if err != nil {
			t.Errorf("reading embedded asset %s: %v", file, err)
		} else if len(body) == 0 {
			t.Errorf("embedded asset %s is empty", file)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking embedded console: %v", err)
	}
	if !found {
		t.Fatal("the Vite build output is empty")
	}
}

func TestHandlerServesConsoleAtRootAndClientRoutes(t *testing.T) {
	handler := Handler()
	if !consoleBuilt(t) {
		for _, route := range []string{"/", "/operations/trips", "/settings/map"} {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("GET %s answered %d, want 503 before the console is built", route, rec.Code)
			}
		}
		return
	}

	for _, route := range []string{"/", "/operations/trips", "/settings/map"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s answered %d, want 200", route, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("GET %s Content-Type = %q, want HTML", route, ct)
		}
		if !strings.Contains(rec.Body.String(), `id="root"`) {
			t.Errorf("GET %s did not serve the React app entry", route)
		}
	}
}

func TestHandlerServesAssetsReferencedByViteIndex(t *testing.T) {
	if !consoleBuilt(t) {
		t.Skip("console assets are not built; run npm run build from console/")
	}

	index, err := content.ReadFile("dist/build/index.html")
	if err != nil {
		t.Fatalf("reading Vite index: %v", err)
	}
	references := regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`).FindAllStringSubmatch(string(index), -1)
	if len(references) < 2 {
		t.Fatalf("Vite index references %d assets, want the JS and CSS bundles", len(references))
	}
	for _, match := range references {
		rec := httptest.NewRecorder()
		Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, match[1], nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s answered %d, want 200", match[1], rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("GET %s is not cached as a fingerprinted asset", match[1])
		}
	}
}

func TestHandlerServesModuleWorkerAsJavaScript(t *testing.T) {
	if !consoleBuilt(t) {
		t.Skip("console assets are not built; run npm run build from console/")
	}

	assets, err := fs.ReadDir(content, "dist/build/assets")
	if err != nil {
		t.Fatalf("reading embedded assets: %v", err)
	}
	for _, asset := range assets {
		if asset.IsDir() || !strings.HasSuffix(asset.Name(), ".mjs") {
			continue
		}
		rec := httptest.NewRecorder()
		Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/"+asset.Name(), nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET module worker answered %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
			t.Errorf("module worker Content-Type = %q, want JavaScript", got)
		}
		return
	}
	t.Fatal("no module worker found in embedded assets")
}

func consoleBuilt(t *testing.T) bool {
	t.Helper()
	_, err := fs.Stat(content, "dist/build/index.html")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("checking embedded console entry: %v", err)
	}
	return err == nil
}

func TestHandlerDoesNotHideMissingAssets(t *testing.T) {
	for _, route := range []string{"/assets/missing.js", "/static/missing.css"} {
		rec := httptest.NewRecorder()
		Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s answered %d, want 404", route, rec.Code)
		}
	}
}
