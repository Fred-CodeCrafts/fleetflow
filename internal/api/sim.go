package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"fleetflow/internal/domain"
	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
	"fleetflow/internal/sim"
)

// tickInterval is how much wall time one simulated step takes while a run is
// live. It paces a run so a viewer can watch it move, rather than the whole
// scenario finishing the instant it starts.
const tickInterval = 50 * time.Millisecond

// maxRunSteps bounds a run that never settles: a vehicle that can never finish
// would otherwise keep the clock going for as long as the process lives.
const maxRunSteps = 200_000

// maxSimVehicles bounds one run, so a single request cannot ask the process to
// build an unbounded number of vehicles.
const maxSimVehicles = 5000

// Error codes for a start request the endpoint refuses.
const (
	codeVehiclesOutOfRange       = "vehicles_out_of_range"
	codeFleetShareOutOfRange     = "fleet_share_out_of_range"
	codeStepOutOfRange           = "step_s_out_of_range"
	codeWindowOutOfRange         = "window_s_out_of_range"
	codeRerouteEveryOutOfRange   = "reroute_every_s_out_of_range"
	codeRerouteThresholdOutOfRan = "reroute_threshold_out_of_range"
)

// simRequest is the body of POST /api/sim/start (DESIGN 3.5). Every field is
// optional: a one-click start sends an empty object and gets the demo defaults.
//
// Only the simulation's own knobs are here. The dynamic cost model is not: the
// FleetFlow arm routes through the server's live engine, which this cannot
// retune without pulling the weights out from under every other caller.
type simRequest struct {
	Seed             *int64   `json:"seed"`
	Vehicles         *int     `json:"vehicles"`
	FleetShare       *float64 `json:"fleet_share"`
	StepS            *float64 `json:"step_s"`
	WindowS          *float64 `json:"window_s"`
	RerouteEveryS    *float64 `json:"reroute_every_s"`
	RerouteThreshold *float64 `json:"reroute_threshold"`
}

// resolve applies the request over the demo defaults and checks the result, so a
// bad run is refused with a named 400 instead of failing deep inside the
// simulator, where the caller could not tell what was wrong.
func (r simRequest) resolve() (sim.Config, error) {
	cfg := sim.DefaultConfig()
	if r.Seed != nil {
		cfg.Seed = *r.Seed
	}
	if r.Vehicles != nil {
		cfg.Vehicles = *r.Vehicles
	}
	if r.FleetShare != nil {
		cfg.FleetShare = *r.FleetShare
	}
	if r.StepS != nil {
		cfg.StepS = *r.StepS
	}
	if r.WindowS != nil {
		cfg.WindowS = *r.WindowS
	}
	if r.RerouteEveryS != nil {
		cfg.RerouteEveryS = *r.RerouteEveryS
	}
	if r.RerouteThreshold != nil {
		cfg.RerouteThreshold = *r.RerouteThreshold
	}

	switch {
	case cfg.Vehicles < 0 || cfg.Vehicles > maxSimVehicles:
		return sim.Config{}, domain.Invalid(codeVehiclesOutOfRange,
			"vehicles must be between 0 and %d", maxSimVehicles)
	case cfg.FleetShare < 0 || cfg.FleetShare > 1:
		return sim.Config{}, domain.Invalid(codeFleetShareOutOfRange,
			"fleet_share must be between 0 and 1")
	case cfg.StepS <= 0:
		return sim.Config{}, domain.Invalid(codeStepOutOfRange, "step_s must be greater than 0")
	case cfg.WindowS < 0:
		return sim.Config{}, domain.Invalid(codeWindowOutOfRange, "window_s must not be negative")
	case cfg.RerouteEveryS < 0:
		return sim.Config{}, domain.Invalid(codeRerouteEveryOutOfRange,
			"reroute_every_s must not be negative")
	case cfg.RerouteThreshold < 0 || cfg.RerouteThreshold >= 1:
		return sim.Config{}, domain.Invalid(codeRerouteThresholdOutOfRan,
			"reroute_threshold must be in [0, 1)")
	}
	return cfg, nil
}

// simStatus is what a start answers with: the run that is now going, so a caller
// can show what it asked for without keeping its own copy.
type simStatus struct {
	Seed       int64   `json:"seed"`
	Vehicles   int     `json:"vehicles"`
	FleetShare float64 `json:"fleet_share"`
	StepS      float64 `json:"step_s"`
	WindowS    float64 `json:"window_s"`
	Running    bool    `json:"running"`
}

// run is one live comparison: the same seeded demand driven twice, once with the
// static baseline engine and once with the dynamic one, stepped together by one
// clock so the two arms cannot drift apart (DESIGN 3.4).
//
// The two arms are separate Sim values, and only the clock goroutine ever steps
// them. Readers take whole snapshots, which the clock publishes after each step,
// so a reader never waits on the simulation's own work.
type run struct {
	cfg sim.Config

	baseline  *sim.Sim
	fleetflow *sim.Sim

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once

	mu     sync.Mutex
	latest sim.Comparison
	steps  int
}

// newRun builds both arms over the same seeded demand. The baseline arm routes on
// the static engine in-process; the FleetFlow arm uses the service it is given,
// which production points at the server's own public API.
func newRun(g *graph.Graph, cfg sim.Config, baseline *engine.Engine, fleet sim.FleetService) (*run, error) {
	baselineArm, err := sim.New(g, cfg)
	if err != nil {
		return nil, err
	}
	fleetArm, err := sim.New(g, cfg)
	if err != nil {
		return nil, err
	}

	baselineArm.SetFleetService(sim.NewEngineFleet(baseline))
	fleetArm.SetFleetService(fleet)

	return &run{
		cfg:       cfg,
		baseline:  baselineArm,
		fleetflow: fleetArm,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}, nil
}

// start sets the clock going and returns at once.
func (r *run) start() { go r.clock() }

// clock advances the run every tick until it is reset, both arms have settled,
// or the step budget runs out.
func (r *run) clock() {
	defer close(r.done)

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
		}

		r.advance()
		if r.steps >= maxRunSteps || r.settled() {
			return
		}
	}
}

// advance steps both arms once and publishes what they now report.
func (r *run) advance() {
	r.baseline.Step()
	r.fleetflow.Step()
	r.steps++

	snapshot := sim.Comparison{
		Baseline:  r.baseline.Collect(),
		FleetFlow: r.fleetflow.Collect(),
	}

	r.mu.Lock()
	r.latest = snapshot
	r.mu.Unlock()
}

// settled reports whether both arms have finished, so the clock can stop on its
// own rather than run until someone resets it.
func (r *run) settled() bool {
	return armSettled(r.baseline) && armSettled(r.fleetflow)
}

// armSettled reports whether one arm has no vehicle still pending or running.
func armSettled(s *sim.Sim) bool {
	for _, v := range s.Vehicles() {
		if state := v.State(); state == sim.VehiclePending || state == sim.VehicleRunning {
			return false
		}
	}
	return true
}

// snapshot returns the metrics of the last completed step.
func (r *run) snapshot() sim.Comparison {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latest
}

// status describes the run for the start response.
func (r *run) status() simStatus {
	return simStatus{
		Seed:       r.cfg.Seed,
		Vehicles:   r.cfg.Vehicles,
		FleetShare: r.cfg.FleetShare,
		StepS:      r.cfg.StepS,
		WindowS:    r.cfg.WindowS,
		Running:    true,
	}
}

// stopRun stops the clock and waits for it to return, or until ctx ends. It is
// safe to call more than once, and on a run that has already finished.
func (r *run) stopRun(ctx context.Context) {
	r.stopOnce.Do(func() { close(r.stop) })
	select {
	case <-r.done:
	case <-ctx.Done():
	}
}

// WithFleetService sets how a run's FleetFlow arm reaches the routing engine.
// Production passes an HTTP client against the server's own public API, so
// simulated traffic is validated, authenticated and measured like an external
// client (PRD US-7). Without it the arm uses the engine in-process.
func WithFleetService(fn func(*engine.Engine) sim.FleetService) Option {
	return func(s *Server) { s.fleetService = fn }
}

// selfFleet is the FleetFlow arm's routing service when its traffic goes over the
// public API. An in-process arm refreshes the weights as it goes, because it holds
// the engine itself; an HTTP client cannot reach the engine behind the API, so
// that refresh is delegated here, through the same engine the API serves from.
// Without it a run over the wire would route on one snapshot for its whole length.
type selfFleet struct {
	sim.FleetService
	eng *engine.Engine
}

// CustomizeEvery implements sim.FleetCustomizer.
func (f selfFleet) CustomizeEvery() time.Duration { return f.eng.CustomizeEvery() }

// DecayStale implements sim.FleetCustomizer.
func (f selfFleet) DecayStale(now time.Time) { f.eng.DecayStale(now) }

// Customize implements sim.FleetCustomizer.
func (f selfFleet) Customize() *engine.Metric { return f.eng.Customize() }

// handleSimStart builds a fresh comparison run from the request and sets its
// clock going, replacing whatever was running before.
func (s *Server) handleSimStart(w http.ResponseWriter, r *http.Request) {
	if s.eng == nil {
		writeError(w, r, errNoEngine)
		return
	}

	var body simRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	cfg, err := body.resolve()
	if err != nil {
		writeError(w, r, err)
		return
	}

	baseline, err := engine.NewBaseline(s.graph, engine.NewDijkstraRouter(s.graph))
	if err != nil {
		writeError(w, r, err)
		return
	}

	fleet := sim.FleetService(sim.NewEngineFleet(s.eng))
	if s.fleetService != nil {
		// The client speaks HTTP, so the clock's weight refresh has to be handed
		// to the engine the server itself routes with.
		fleet = selfFleet{FleetService: s.fleetService(s.eng), eng: s.eng}
	}

	live, err := newRun(s.graph, cfg, baseline, fleet)
	if err != nil {
		writeError(w, r, err)
		return
	}

	s.swapRun(r.Context(), live)
	live.start()

	writeJSON(w, http.StatusAccepted, live.status())
}

// handleSimReset stops the run and clears it. It answers 204 whether or not one
// was running, because "there is no run now" is true either way.
func (s *Server) handleSimReset(w http.ResponseWriter, r *http.Request) {
	s.stopRun(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// swapRun replaces the running comparison, stopping whatever was there first.
// Stopping before starting means the two clocks never overlap.
func (s *Server) swapRun(ctx context.Context, next *run) {
	s.runMu.Lock()
	previous := s.run
	s.run = next
	s.runMu.Unlock()

	if previous != nil {
		previous.stopRun(ctx)
	}
}

// stopRun stops the running comparison, if any, and clears it.
func (s *Server) stopRun(ctx context.Context) {
	s.runMu.Lock()
	previous := s.run
	s.run = nil
	s.runMu.Unlock()

	if previous != nil {
		previous.stopRun(ctx)
	}
}
