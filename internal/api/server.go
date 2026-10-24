// Package api serves FleetFlow's fleet operations and OSRM routing API.
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

	"fleetflow/internal/auth"
	"fleetflow/internal/domain"
	"fleetflow/internal/driver"
	"fleetflow/internal/osrm"
	"fleetflow/internal/simulation"
	"fleetflow/internal/trip"
	"fleetflow/internal/vehicle"
	"fleetflow/web"
)

const (
	readinessTimeout  = 2 * time.Second
	shutdownTimeout   = 10 * time.Second
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 15 * time.Second
	idleTimeout       = 60 * time.Second
	requestTimeout    = 10 * time.Second
)

type StopFunc func(context.Context) error

var errNoFleet = domain.Unavailable("fleet_unavailable", "fleet records are not configured")
var errNoAuth = domain.Unavailable("auth_unavailable", "API keys are not configured")

type Server struct {
	vehicles   *vehicle.Service
	drivers    *driver.Service
	trips      *trip.Service
	router     osrm.Router
	simulation *simulation.Service
	keys       *auth.Service
	live       *liveVehicles

	mux      *http.ServeMux
	handler  http.Handler
	logger   *slog.Logger
	limiter  *rateLimiter
	dbCheck  func(context.Context) error
	stoppers []StopFunc
}

type Option func(*Server)

func WithVehicles(svc *vehicle.Service) Option {
	return func(s *Server) { s.vehicles = svc }
}

func WithDrivers(svc *driver.Service) Option {
	return func(s *Server) { s.drivers = svc }
}

func WithTrips(svc *trip.Service) Option {
	return func(s *Server) { s.trips = svc }
}

func WithSimulation(svc *simulation.Service) Option {
	return func(s *Server) {
		if svc == nil {
			return
		}
		s.simulation = svc
		s.stoppers = append(s.stoppers, svc.Shutdown)
	}
}

func WithRouter(router osrm.Router) Option {
	return func(s *Server) { s.router = router }
}

func WithAPIKeys(svc *auth.Service) Option {
	return func(s *Server) { s.keys = svc }
}

func WithDatabaseCheck(check func(context.Context) error) Option {
	return func(s *Server) { s.dbCheck = check }
}

func WithStopFunc(fn StopFunc) Option {
	return func(s *Server) { s.stoppers = append(s.stoppers, fn) }
}

func WithLogger(logger *slog.Logger) Option {
	return func(s *Server) {
		if logger != nil {
			s.logger = logger
		}
	}
}

func WithRateLimit(perSecond, burst float64) Option {
	return func(s *Server) {
		if perSecond > 0 && burst > 0 {
			s.limiter = newRateLimiter(perSecond, burst)
		}
	}
}

func NewServer(opts ...Option) *Server {
	s := &Server{
		mux:     http.NewServeMux(),
		logger:  slog.Default(),
		limiter: newRateLimiter(ratePerSecond, rateBurst),
		live:    newLiveVehicles(liveVehicleTTL, maxLiveVehicles),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.routes()
	s.handler = withRequestID(withLogging(s.logger, s.mux))
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.Handle("GET /readyz", withTimeout(requestTimeout, http.HandlerFunc(s.handleReadyz)))
	s.mux.HandleFunc("GET /api/healthz", s.handleHealthz)
	s.mux.Handle("GET /api/readyz", withTimeout(requestTimeout, http.HandlerFunc(s.handleReadyz)))

	s.mux.Handle("POST /api/locations", s.authed(s.handleCreateLocation))
	s.mux.Handle("GET /api/locations", s.public(s.handleListLocations))
	s.mux.Handle("POST /api/route", s.public(s.handleRoute))

	s.mux.Handle("GET /api/vehicles", s.public(s.handleListVehicles))
	s.mux.Handle("POST /api/vehicles", s.public(s.handleCreateVehicle))
	s.mux.Handle("GET /api/drivers", s.public(s.handleListDrivers))
	s.mux.Handle("POST /api/drivers", s.public(s.handleCreateDriver))
	s.mux.Handle("PUT /api/drivers/{id}/vehicle", s.public(s.handleAssignDriverVehicle))
	s.mux.Handle("POST /api/drivers/{id}/vehicle", s.public(s.handleAssignDriverVehicle))
	s.mux.Handle("GET /api/trips", s.public(s.handleListTrips))
	s.mux.Handle("POST /api/trips", s.public(s.handleCreateTrip))
	s.mux.Handle("PATCH /api/trips/{id}/status", s.public(s.handleUpdateTripStatus))
	s.mux.Handle("POST /api/simulation/runs", s.public(s.handleCreateSimulationRun))
	s.mux.Handle("GET /api/simulation/runs", s.public(s.handleListSimulationRuns))
	s.mux.Handle("GET /api/simulation/runs/{id}", s.public(s.handleGetSimulationRun))

	console := s.public(web.Handler().ServeHTTP)
	s.mux.Handle("GET /{$}", console)
	s.mux.Handle("GET /assets/", console)
	s.mux.Handle("GET /operations", console)
	s.mux.Handle("GET /operations/", console)
	s.mux.Handle("GET /resources", console)
	s.mux.Handle("GET /resources/", console)
	s.mux.Handle("GET /settings", console)
	s.mux.Handle("GET /settings/", console)
}

func (s *Server) public(h http.HandlerFunc) http.Handler {
	return s.limiter.middleware(withTimeout(requestTimeout, h))
}

func (s *Server) authed(h http.HandlerFunc) http.Handler {
	return s.limiter.middleware(withTimeout(requestTimeout, s.requireAPIKey(h)))
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

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

func (s *Server) ListenAndServe(addr string) error {
	return s.Run(context.Background(), addr)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	simulationStatus := "not_configured"
	if s.simulation != nil {
		simulationStatus = "available"
	}
	body := map[string]any{"routing": "not_configured", "simulation": simulationStatus}
	ready := false
	if s.router != nil {
		if checker, ok := s.router.(osrm.HealthChecker); ok {
			ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
			defer cancel()
			if err := checker.Health(ctx); err != nil {
				body["routing"] = "unavailable"
			} else {
				body["routing"] = "available"
				ready = true
			}
		} else {
			body["routing"] = "configured"
			ready = true
		}
	}

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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
