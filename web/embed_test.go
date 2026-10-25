package web

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
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

// The console's assets are immutable and the map chunk alone is a megabyte, so
// they are served gzipped when the client asks for it. The bytes must survive
// the round trip, and a client that declines gzip must still get the file.
func TestAssetsAreServedGzippedOnlyWhenAccepted(t *testing.T) {
	if !consoleBuilt(t) {
		t.Skip("console assets are not built; run npm run build from console/")
	}

	asset := largestJavaScriptAsset(t)
	if asset == "" {
		t.Skip("no JavaScript asset in the build to test compression against")
	}

	plain := httptest.NewRecorder()
	Handler().ServeHTTP(plain, httptest.NewRequest(http.MethodGet, asset, nil))
	if plain.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d, want 200", asset, plain.Code)
	}
	if encoding := plain.Header().Get("Content-Encoding"); encoding != "" {
		t.Fatalf("uncompressed request was answered with %q", encoding)
	}

	request := httptest.NewRequest(http.MethodGet, asset, nil)
	request.Header.Set("Accept-Encoding", "gzip, deflate")
	packed := httptest.NewRecorder()
	Handler().ServeHTTP(packed, request)
	if packed.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d, want 200", asset, packed.Code)
	}
	if encoding := packed.Header().Get("Content-Encoding"); encoding != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", encoding)
	}
	if vary := packed.Header().Get("Vary"); vary != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", vary)
	}
	if packed.Body.Len() >= plain.Body.Len() {
		t.Errorf("gzip body is %d bytes, uncompressed is %d; compression is not helping", packed.Body.Len(), plain.Body.Len())
	}

	reader, err := gzip.NewReader(packed.Body)
	if err != nil {
		t.Fatalf("gzip body does not decompress: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading the decompressed body: %v", err)
	}
	if !bytes.Equal(decoded, plain.Body.Bytes()) {
		t.Fatal("the compressed asset does not decompress to the asset on disk")
	}

	declined := httptest.NewRequest(http.MethodGet, asset, nil)
	declined.Header.Set("Accept-Encoding", "gzip;q=0")
	answer := httptest.NewRecorder()
	Handler().ServeHTTP(answer, declined)
	if encoding := answer.Header().Get("Content-Encoding"); encoding != "" {
		t.Fatalf("a client that declined gzip was answered with %q", encoding)
	}
}

// largestJavaScriptAsset returns the largest fingerprinted .js file in the build,
// which is the map chunk and therefore the one worth compressing.
func largestJavaScriptAsset(t *testing.T) string {
	t.Helper()
	var largest string
	var largestSize int
	err := fs.WalkDir(content, "dist/build/assets", func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(file, ".js") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if int(info.Size()) > largestSize {
			largest, largestSize = file, int(info.Size())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the built assets: %v", err)
	}
	return strings.TrimPrefix(largest, "dist/build")
}
