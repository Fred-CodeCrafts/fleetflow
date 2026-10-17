// Package api serves the FleetFlow HTTP API: vehicle reports, route queries,
// simulation control, metrics and the live event stream.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"fleetflow/internal/graph"
)

const (
	// readinessTimeout bounds the database probe used by /readyz.
	readinessTimeout = 2 * time.Second
	// shutdownTimeout bounds the drain of in-flight requests on shutdown.
	shutdownTimeout = 10 * time.Second
	// readHeaderTimeout bounds how long a client may take to send its headers.
	readHeaderTimeout = 10 * time.Second
)

// StopFunc stops one background component, such as the customizer or a running
// simulation. It runs during shutdown, before the listener drains.
type StopFunc func(context.Context) error

// Server holds the API handlers and everything they need. It is an
// http.Handler, so it can be mounted anywhere.
type Server struct {
	graph    *graph.Graph
	mux      *http.ServeMux
	dbCheck  func(context.Context) error
	stoppers []StopFunc
}

// Option customises a Server at construction time.
type Option func(*Server)

// WithDatabaseCheck registers the readiness probe for the database. Without it,
// /readyz reports the database as not configured.
func WithDatabaseCheck(check func(context.Context) error) Option {
	return func(s *Server) { s.dbCheck = check }
}

// WithStopFunc registers a background component to stop during graceful
// shutdown.
func WithStopFunc(fn StopFunc) Option {
	return func(s *Server) { s.stoppers = append(s.stoppers, fn) }
}

// NewServer builds the API handler for a road graph.
func NewServer(g *graph.Graph, opts ...Option) *Server {
	s := &Server{graph: g, mux: http.NewServeMux()}
	for _, opt := range opts {
		opt(s)
	}
	s.routes()
	return s
}

// routes registers every endpoint.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Run serves HTTP on addr until ctx is cancelled. On cancellation it stops the
// registered components and drains in-flight requests, then returns. It returns
// an error only when the server stops for a reason other than ctx ending.
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	// The context ending is the signal to stop, so its deadline gives us nothing
	// to finish with: give shutdown its own budget.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	for _, stop := range s.stoppers {
		if err := stop(shutdownCtx); err != nil {
			log.Printf("shutdown: stopping a component failed: %v", err)
		}
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	return nil
}

// ListenAndServe runs the server on addr and blocks until it stops. It has no
// shutdown signal; use Run when the process needs to drain on SIGTERM.
func (s *Server) ListenAndServe(addr string) error {
	return s.Run(context.Background(), addr)
}

// handleHealthz reports that the process is up.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz reports whether the process can serve traffic: the graph is
// loaded and, when a database is configured, it is reachable. It answers 503
// when either check fails, so a load balancer stops sending traffic.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{"graph_loaded": s.graph != nil}
	ready := s.graph != nil

	if s.dbCheck == nil {
		body["database"] = "not_configured"
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		if err := s.dbCheck(ctx); err != nil {
			body["database"] = "unreachable"
			ready = false
		} else {
			body["database"] = "reachable"
		}
	}

	if !ready {
		body["status"] = "unavailable"
		writeJSON(w, http.StatusServiceUnavailable, body)
		return
	}
	body["status"] = "ok"
	writeJSON(w, http.StatusOK, body)
}

// writeJSON writes v as a JSON body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
