// Package api serves the FleetFlow HTTP API: vehicle reports, route queries,
// simulation control, metrics and the live event stream.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"fleetflow/internal/auth"
	"fleetflow/internal/domain"
	"fleetflow/internal/driver"
	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
	"fleetflow/internal/sim"
	"fleetflow/internal/trip"
	"fleetflow/internal/vehicle"
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

// errNoFleet is reported by a record endpoint that is reachable without its
// service behind it, which is the case when the process runs without a database
// (a memory-free run or a unit test). The endpoint exists but cannot serve.
var errNoFleet = domain.Unavailable("fleet_unavailable", "fleet records are not configured")

// errNoAuth is reported by an ingest endpoint whose key store is not configured.
// It serves rather than refusing every key, because "I cannot check keys" and
// "your key is wrong" are different answers and only one of them is the client's
// to act on.
var errNoAuth = domain.Unavailable("auth_unavailable", "API keys are not configured")

// Server holds the API handlers and everything they need. It is an
// http.Handler, so it can be mounted anywhere.
type Server struct {
	graph *graph.Graph
	// eng is the live routing state: reports fold into it, route queries read
	// its snapshot. It is nil when the server is built without WithEngine, which
	// leaves the public endpoints answering 503.
	eng *engine.Engine
	// vehicles backs the record endpoints for vehicles. It is nil when the server
	// is built without WithVehicles, which leaves those endpoints answering 503
	// rather than dereferencing a nil service.
	vehicles *vehicle.Service
	// drivers backs the driver endpoints, including assignment. Nil when built
	// without WithDrivers.
	drivers *driver.Service
	// trips backs the trip endpoints. Nil when built without WithTrips.
	trips *trip.Service
	// keys authenticates the API key the ingest endpoints require. Nil when the
	// server is built without WithAPIKeys; those endpoints then refuse every
	// request, because an ingest endpoint that cannot check its key must not
	// serve.
	keys *auth.Service
	mux  *http.ServeMux
	// run is the live comparison, or nil when none is running. Only its own
	// clock goroutine steps the arm simulators, so runMu guards the pointer and
	// nothing else.
	runMu sync.Mutex
	run   *run
	// fleetService builds the FleetService a run's FleetFlow arm uses. Nil means
	// the engine in-process; production sets an HTTP client against the server's
	// own public API.
	fleetService func(*engine.Engine) sim.FleetService
	// handler is the mux wrapped in the shared middleware, built once so every
	// request pays for the chain only in the handler, not in ServeHTTP.
	handler http.Handler
	logger  *slog.Logger
	limiter *rateLimiter
	// internalKey is the key the process presents for its own simulated traffic,
	// or empty when there is none. It is matched exactly and only to exempt that
	// one client from the public rate limit.
	internalKey string
	// live is where real vehicles are last seen, so the dashboard can show them
	// beside simulated ones. It is always present; an empty register is a valid
	// state, not a missing dependency.
	live *liveVehicles
	// streams counts the clients holding the event stream open, so a run can be
	// tied to somebody watching it (T44).
	streams  *streamClients
	dbCheck  func(context.Context) error
	stoppers []StopFunc
}

// Option customises a Server at construction time.
type Option func(*Server)

// WithEngine gives the server the routing engine that reports write to and route
// queries read from. Without it the public endpoints answer 503; the rest of the
// API works either way.
//
// The engine must have been built over this server's graph: a route path carries
// edge indices into that graph, and the server translates them with its own.
func WithEngine(e *engine.Engine) Option {
	return func(s *Server) { s.eng = e }
}

// WithVehicles gives the server the vehicle service the record endpoints read
// and write through. Without it those endpoints answer 503.
func WithVehicles(svc *vehicle.Service) Option {
	return func(s *Server) { s.vehicles = svc }
}

// WithDrivers gives the server the driver service, including assignment. Without
// it the driver endpoints answer 503.
func WithDrivers(svc *driver.Service) Option {
	return func(s *Server) { s.drivers = svc }
}

// WithTrips gives the server the trip service. Without it the trip endpoints
// answer 503.
func WithTrips(svc *trip.Service) Option {
	return func(s *Server) { s.trips = svc }
}

// WithAPIKeys gives the server the key service the ingest endpoints authenticate
// against. Without it those endpoints answer 503 rather than serving, because an
// endpoint that cannot check the key it is supposed to require must not publish.
func WithAPIKeys(svc *auth.Service) Option {
	return func(s *Server) { s.keys = svc }
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

// WithRateLimit replaces the public endpoints' per-client allowance. It is for a
// caller whose traffic is not a person at a dashboard: the simulator sends its
// reports and route queries through these same endpoints (DESIGN 3.4), and a demo
// run makes far more requests per wall second than any operator ever does, so on
// the default limit it would throttle itself.
//
// Non-positive values are ignored, because a zero rate would divide by zero in
// the limiter rather than mean "no limit". A caller that wants the limit out of
// the way passes a rate far above what it can produce.
func WithRateLimit(perSecond, burst float64) Option {
	return func(s *Server) {
		if perSecond <= 0 || burst <= 0 {
			return
		}
		s.limiter = newRateLimiter(perSecond, burst)
	}
}

// WithInternalKey names the API key the process uses for its own simulator. A
// request bearing it is not rate limited, because that client is this process and
// the limit is sized for real ones: a simulation compresses time, so it reports
// far faster than any real fleet ever could while staying a single trusted
// caller. The key is still authenticated like any other.
func WithInternalKey(key string) Option {
	return func(s *Server) { s.internalKey = key }
}

// NewServer builds the API handler for a road graph.
func NewServer(g *graph.Graph, opts ...Option) *Server {
	s := &Server{
		graph:   g,
		mux:     http.NewServeMux(),
		logger:  slog.Default(),
		limiter: newRateLimiter(ratePerSecond, rateBurst),
		live:    newLiveVehicles(liveVehicleTTL, maxLiveVehicles),
		streams: newStreamClients(),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.routes()

	// A live simulation is a background component like the customizer, so it
	// stops with the rest: shutdown stops the run and waits for its clock.
	s.stoppers = append(s.stoppers, func(ctx context.Context) error {
		s.stopRun(ctx)
		return nil
	})

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

	// The two ingest endpoints are the ones an outside system calls, so they are
	// the ones that require an API key (DESIGN 3.5, PRD US-13). The fleet record
	// endpoints stay open: they belong to the dashboard, and the demo is meant to
	// be browsable.
	s.mux.Handle("POST /api/reports", s.authed(s.handleReport))
	s.mux.Handle("POST /api/route", s.authed(s.handleRoute))

	// Simulation control (DESIGN 3.5). These belong to the dashboard, like the
	// record endpoints, so they are public; the traffic a run produces goes
	// through the keyed ingest endpoints above.
	s.mux.Handle("POST /api/sim/start", s.public(s.handleSimStart))
	s.mux.Handle("POST /api/sim/reset", s.public(s.handleSimReset))

	// The metrics the dashboard's strip reads. Public for the same reason: it is
	// read by a browser, and it discloses no more than the panels already show.
	s.mux.Handle("GET /api/metrics", s.public(s.handleMetrics))

	// The event stream. It takes the rate limit but not the request deadline: a
	// response that is meant to stay open cannot be given one (T43).
	s.mux.Handle("GET /api/stream", s.stream(s.handleStream))

	// Fleet records (DESIGN 3.5). These are the endpoints that touch PostgreSQL;
	// the two above never do.
	s.mux.Handle("GET /api/vehicles", s.public(s.handleListVehicles))
	s.mux.Handle("POST /api/vehicles", s.public(s.handleCreateVehicle))
	s.mux.Handle("POST /api/vehicles/{id}/driver", s.public(s.handleAssignDriver))

	s.mux.Handle("GET /api/drivers", s.public(s.handleListDrivers))
	s.mux.Handle("POST /api/drivers", s.public(s.handleCreateDriver))

	s.mux.Handle("GET /api/trips", s.public(s.handleListTrips))
	s.mux.Handle("POST /api/trips", s.public(s.handleCreateTrip))
	s.mux.Handle("POST /api/trips/{id}/status", s.public(s.handleSetTripStatus))
}

// public wraps a handler with what every public endpoint shares: the per-client
// rate limit, then a request deadline. Both are applied in this one place, so an
// endpoint cannot be published without them (T36b).
func (s *Server) public(h http.HandlerFunc) http.Handler {
	// The limiter is outermost, so a client over its allowance is refused before
	// a deadline it does not need is created for the request.
	return s.rateLimited(withTimeout(requestTimeout, h))
}

// authed wraps an endpoint that requires a valid API key. The layers are public's
// with the key check added innermost: an unauthenticated caller is still a caller
// that can be too fast or too slow, so the rate limit and the deadline come first,
// and the key lookup itself is bounded by that deadline.
func (s *Server) authed(h http.HandlerFunc) http.Handler {
	return s.rateLimited(withTimeout(requestTimeout, s.requireAPIKey(h)))
}

// rateLimited applies the public per-client limit to a handler, with one
// exception: a request bearing this process's own key. That client is the
// simulator, which runs here and compresses time, so the limit — sized to keep a
// real dashboards and reporting fleet comfortable — would only make the server
// throttle itself (DESIGN 3.4). Every other caller, authenticated or not, is
// limited as before; the exception is an exact match, not a bypass.
func (s *Server) rateLimited(next http.Handler) http.Handler {
	limited := s.limiter.middleware(next)
	if s.internalKey == "" {
		return limited
	}

	internal := []byte(s.internalKey)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if presented, ok := bearerKey(r); ok &&
			subtle.ConstantTimeCompare([]byte(presented), internal) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		limited.ServeHTTP(w, r)
	})
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
