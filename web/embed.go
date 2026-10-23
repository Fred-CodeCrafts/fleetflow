// Package web serves the production console built by Vite and embedded in the
// FleetFlow binary.
package web

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// content is the Vite output. Rebuild console/ before compiling after a UI change.
//
//go:embed all:dist
var content embed.FS

// Handler serves the embedded console and its fingerprinted assets.
func Handler() http.Handler { return http.HandlerFunc(serve) }

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

	file := "dist/" + requestPath
	info, err := fs.Stat(content, file)
	if err == nil && !info.IsDir() {
		writeAsset(w, requestPath, file)
		return
	}

	// Missing fingerprinted assets and paths with extensions are real 404s.
	// Other paths are client-side routes and need the SPA entry document.
	if strings.HasPrefix(requestPath, "assets/") || path.Ext(requestPath) != "" ||
		(err != nil && !errors.Is(err, fs.ErrNotExist)) {
		http.NotFound(w, r)
		return
	}
	writeAsset(w, "index.html", "dist/index.html")
}

func writeAsset(w http.ResponseWriter, name, file string) {
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
	_, _ = w.Write(body)
}
