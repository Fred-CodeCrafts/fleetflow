// Package web serves the production console built by Vite and embedded in the
// FleetFlow binary.
package web

import (
	"bytes"
	"compress/gzip"
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

// content is the Vite output. Rebuild console/ before compiling after a UI change.
//
//go:embed all:dist
var content embed.FS

// Handler serves the embedded console and its fingerprinted assets.
func Handler() http.Handler { return http.HandlerFunc(serve) }

// compressed holds one gzip copy of every asset worth compressing.
//
// The assets are immutable and embedded, so compressing them per request would
// repeat identical work for every visitor; the map chunk alone is a megabyte. One
// pass at first use costs a few milliseconds and turns each response into a slice
// of memory.
var compressed = sync.OnceValue(func() map[string][]byte {
	packed := make(map[string][]byte)
	_ = fs.WalkDir(content, "dist", func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil //nolint:nilerr // a walk error must not break serving the console
		}
		name := strings.TrimPrefix(file, "dist/build/")
		body, err := content.ReadFile(file)
		if err != nil || !compressible(name) || len(body) < compressMinBytes {
			return nil
		}
		var buffer bytes.Buffer
		writer, err := gzip.NewWriterLevel(&buffer, gzip.BestCompression)
		if err != nil {
			return nil //nolint:nilerr // no compression is a valid fallback
		}
		if _, err := writer.Write(body); err != nil {
			return nil //nolint:nilerr // no compression is a valid fallback
		}
		if err := writer.Close(); err != nil {
			return nil //nolint:nilerr // no compression is a valid fallback
		}
		if buffer.Len() < len(body) {
			packed[name] = buffer.Bytes()
		}
		return nil
	})
	return packed
})

// compressMinBytes is the size below which gzip overhead is not worth the header.
const compressMinBytes = 1024

// compressible reports whether an asset is text that gzip shrinks. Images and
// fonts are already compressed.
func compressible(name string) bool {
	switch path.Ext(name) {
	case ".js", ".mjs", ".css", ".html", ".svg", ".json", ".geojson", ".map":
		return true
	}
	return false
}

func serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestPath := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(r.URL.Path, "/")), "/")
	if requestPath == "" || requestPath == "." {
		requestPath = "index.html"
	}

	file := "dist/build/" + requestPath
	info, err := fs.Stat(content, file)
	if err == nil && !info.IsDir() {
		writeAsset(w, r, requestPath, file)
		return
	}

	// Missing fingerprinted assets and paths with extensions are real 404s.
	// Other paths are client-side routes and need the SPA entry document.
	if requestPath != "index.html" &&
		(strings.HasPrefix(requestPath, "assets/") || path.Ext(requestPath) != "") ||
		(err != nil && !errors.Is(err, fs.ErrNotExist)) {
		http.NotFound(w, r)
		return
	}
	writeAsset(w, r, "index.html", "dist/build/index.html")
}

func writeAsset(w http.ResponseWriter, r *http.Request, name, file string) {
	body, err := content.ReadFile(file)
	if err != nil {
		if name == "index.html" && errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "console is not built; run npm run build from console/", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "console asset unavailable", http.StatusInternalServerError)
		return
	}

	switch path.Ext(name) {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".js", ".mjs":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	if packed, ok := compressed()[name]; ok && acceptsGzip(r) {
		// Vary matters: a cache that stored the compressed body must not hand it
		// to a client that did not ask for it.
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Set("Content-Length", strconv.Itoa(len(packed)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(packed)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

// acceptsGzip reports whether the client asked for gzip, including the
// "gzip;q=0" form that declines it.
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		encoding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(encoding), "gzip") {
			continue
		}
		quality := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
		return quality != "q=0" && quality != "q=0.0" && quality != "q=0.00" && quality != "q=0.000"
	}
	return false
}
