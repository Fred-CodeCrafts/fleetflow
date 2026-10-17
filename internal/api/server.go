// Package api serves the FleetFlow HTTP API: vehicle reports, route queries,
// simulation control, metrics and the live event stream.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"time"

	"fleetflow/internal/domain"
	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
)

const (
	// readinessTimeout bounds the database probe used by /readyz.
	readinessTimeout = 2 * time.Second
	// shutdownTimeout bounds the drain of in-flight requests on shutdown.
	shutdownTimeout = 10 * time.Second
	// readHeaderTimeout bounds how long a client may take to send its headers.
	readHeaderTimeout = 10 * time.Second
	// readTimeout bounds reading a whole request, body included. It applies to the
	// request only, so it is safe for the event stream, which is a long response.
	readTimeout = 15 * time.Second
	// idleTimeout bounds how long a keep-alive connection may sit unused before it
	// is closed, so idle clients cannot hold connections open.
	idleTimeout = 60 * time.Second
	// requestTimeout is the deadline given to a request that any client may send.
	// It is what turns a stuck dependency into the 503 that domain.Unavailable
	// already maps to, instead of a connection held open indefinitely.
	requestTimeout = 10 * time.Second
)

// StopFunc stops one background component, such as the customizer or a running
// simulation. It runs during shutdown, before the listener drains.
type StopFunc func(context.Context) error

// errNoEngine is reported by a public endpoint that is reachable without an
// engine behind it. The endpoint exists but cannot serve, which is exactly what
// unavailable means.
var errNoEngine = domain.Unavailable("engine_unavailable", "the routing engine is not configured")

// Server holds the API handlers and everything they need. It is an
// http.Handler, so it can be mounted anywhere.
type Server struct {
	graph *graph.Graph
	// eng is the live routing state: reports fold into it, route queries read
	// its snapshot. It is nil when the server is built without WithEngine, which
	// leaves the public endpoints answering 503.
	eng *engine.Engine
	mux *http.ServeMux
	// handler is the mux wrapped in the shared middleware, built once so every
	// request pays for the chain only in the handler, not in ServeHTTP.
	handler  http.Handler
	logger   *slog.Logger
	limiter  *rateLimiter
	dbCheck  func(context.Context) error
	stoppers []StopFunc
}

// Option customises a Server at construction time.
type Option func(*Server)

// WithEngine gives the server the routing engine that reports write to and route
// queries read from. Without it the public endpoints answer 503; the rest of the
// API works either way.
func WithEngine(e *engine.Engine) Option {
	return func(s *Server) { s.eng = e }
}

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

// WithLogger sets the logger used for the per-request lines. The default is
// slog.Default(); a nil logger is ignored so the default stays in place.
func WithLogger(logger *slog.Logger) Option {
	return func(s *Server) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// NewServer builds the API handler for a road graph.
func NewServer(g *graph.Graph, opts ...Option) *Server {
	s := &Server{
		graph:   g,
		mux:     http.NewServeMux(),
		logger:  slog.Default(),
		limiter: newRateLimiter(ratePerSecond, rateBurst),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.routes()

	// Request ID first, so the logging line and every error body can quote it.
	s.handler = withRequestID(withLogging(s.logger, s.mux))
	return s
}

// routes registers every endpoint.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	// Readiness may touch the database, so it gets a deadline of its own: a probe
	// that hangs is worse than one that reports unready.
	s.mux.Handle("GET /readyz", withTimeout(requestTimeout, http.HandlerFunc(s.handleReadyz)))

	s.mux.Handle("POST /api/reports", s.public(s.handleReport))
}

// public wraps a handler with what every public endpoint shares: the per-client
// rate limit, then a request deadline. Both are applied in this one place, so an
// endpoint cannot be published without them (T36b).
func (s *Server) public(h http.HandlerFunc) http.Handler {
	// The limiter is outermost, so a client over its allowance is refused before
	// a deadline it does not need is created for the request.
	return s.limiter.middleware(withTimeout(requestTimeout, h))
}

// newHTTPServer builds the listener's settings.
//
// WriteTimeout is deliberately left unset: the event stream is a long-lived
// response, and a write deadline would sever it mid-stream. ReadHeaderTimeout,
// ReadTimeout and IdleTimeout bound what a client may send and how long it may
// sit idle, which is where a slow client could otherwise hold resources.
func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// Run serves HTTP on addr until ctx is cancelled. On cancellation it stops the
// registered components and drains in-flight requests, then returns. It returns
// an error only when the server stops for a reason other than ctx ending.
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := newHTTPServer(addr, s)

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
