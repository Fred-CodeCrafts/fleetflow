// Package web serves the FleetFlow dashboard: one page of static HTML and CSS,
// embedded into the binary so a deployment ships no asset directory and no build
// step stands between editing a file and seeing it in a browser (DESIGN 3.6).
//
// The package lives beside its assets because go:embed cannot name a file
// outside its own directory. Everything here is standard library: no framework,
// no bundler, no downloaded font, so the dashboard works offline and on a host
// with no network at all.
package web

import (
	"embed"
	"net/http"
)

// content is the embedded dashboard.
//
//go:embed index.html style.css app.js
var content embed.FS

// asset is one URL the dashboard answers with: the file behind it, and the type
// to serve it as.
type asset struct {
	path  string
	file  string
	ctype string
}

// served lists the whole dashboard. The content type is named here rather than
// guessed from the extension: three files are few enough to be explicit about,
// and a guess would depend on the mime database of whatever host is running,
// which a Windows development machine and a scratch container disagree on.
//
// The assets sit under one prefix and the page is matched exactly, which is what
// lets the server mount the dashboard without a catch-all pattern: see the note
// at the route in internal/api/server.go.
var served = []asset{
	{path: "/", file: "index.html", ctype: "text/html; charset=utf-8"},
	{path: "/static/style.css", file: "style.css", ctype: "text/css; charset=utf-8"},
	{path: "/static/app.js", file: "app.js", ctype: "text/javascript; charset=utf-8"},
}

// Handler returns the dashboard: the page at the root, and each asset at its own
// path. Anything else is a 404, so a mistyped asset is visible in the browser
// rather than quietly answered with the page.
func Handler() http.Handler { return http.HandlerFunc(serve) }

// serve writes one asset, or a 404 for a path the dashboard does not have.
func serve(w http.ResponseWriter, r *http.Request) {
	for _, a := range served {
		if a.path != r.URL.Path {
			continue
		}

		body, err := content.ReadFile(a.file)
		if err != nil {
			// Unreachable while the embed directive and this table agree;
			// embed_test.go is what keeps them agreeing.
			http.Error(w, "dashboard asset missing", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", a.ctype)
		// The assets are compiled in, so a changed page means a new binary.
		// Asking the browser to revalidate costs one small request and removes
		// the case where a cached page is shown against a newer API.
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(body)
		return
	}

	http.NotFound(w, r)
}
