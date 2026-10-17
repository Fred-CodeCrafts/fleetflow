// Package api serves the FleetFlow HTTP API: vehicle reports, route queries,
// simulation control, metrics and the live event stream.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"fleetflow/internal/graph"
)

// Server holds the API handlers and everything they need. It is an
// http.Handler, so it can be mounted anywhere.
type Server struct {
	graph *graph.Graph
	mux   *http.ServeMux
}

// NewServer builds the API handler for a road graph.
func NewServer(g *graph.Graph) *Server {
	s := &Server{graph: g, mux: http.NewServeMux()}
	s.routes()
	return s
}

// routes registers every endpoint.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the server on addr and blocks until it stops.
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}

// handleHealthz reports that the process is up.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeJSON writes v as a JSON body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error body.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
