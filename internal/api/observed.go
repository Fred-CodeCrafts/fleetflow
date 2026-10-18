package api

import (
	"log/slog"
	"sync"
	"time"

	"fleetflow/internal/engine"
	"fleetflow/internal/sim"
)

// observedFleet is the FleetFlow arm's routing service with its traffic counted and
// its failures reported.
//
// It exists because the arm's most important property -- that its traffic really
// crosses the wire -- is otherwise invisible. A run whose HTTP calls all failed
// still completes both arms and reports trip times; the numbers are simply measured
// on weights nothing ever updated. Counting the calls and naming the first failure
// is what tells a good run from a silently empty one.
//
// Only the first failure of each kind is logged. A run with a broken API fails on
// every vehicle of every tick, so logging each one would bury the cause in tens of
// thousands of identical lines; the totals are reported when the run ends, and the
// first line of each kind carries the detail needed to fix it.
type observedFleet struct {
	inner  sim.FleetService
	logger *slog.Logger

	mu sync.Mutex
	// routes and reports are the calls that succeeded, so a run can state how much
	// traffic it actually produced.
	routes  int
	reports int
	// routeFailures and reportFailures are every failed call, which is the number
	// the run's own metrics cannot see: sim counts a refused report, but not the
	// route it never got.
	routeFailures  int
	reportFailures int
	// loggedRoute and loggedReport record that the detail for that kind has been
	// written, so it is written once.
	loggedRoute  bool
	loggedReport bool
}

// observedFleetOf wraps inner, or returns it untouched when there is no logger to
// report to.
func observedFleetOf(inner sim.FleetService, logger *slog.Logger) sim.FleetService {
	if logger == nil {
		return inner
	}

	counted := &observedFleet{inner: inner, logger: logger}
	// The clock asks its service for weight refresh by type assertion, so the
	// wrapper must answer that assertion exactly as the thing it wraps does. A
	// service that is not a customizer stays not one: claiming otherwise would
	// make the clock refresh on a zero interval.
	if customizer, ok := inner.(sim.FleetCustomizer); ok {
		return &observedCustomFleet{observedFleet: counted, customizer: customizer}
	}
	return counted
}

// Route implements sim.FleetService.
func (f *observedFleet) Route(fromID, toID string, held []int) (engine.RouteAnswer, error) {
	answer, err := f.inner.Route(fromID, toID, held)

	f.mu.Lock()
	defer f.mu.Unlock()

	if err == nil {
		f.routes++
		return answer, nil
	}

	f.routeFailures++
	if !f.loggedRoute {
		f.loggedRoute = true
		f.logger.Error("a simulated route query failed",
			slog.String("from", fromID),
			slog.String("to", toID),
			slog.Bool("priced_held_path", held != nil),
			slog.String("error", err.Error()),
			slog.String("note", "the vehicle is left unroutable; further route failures are counted, not logged"))
	}
	return answer, err
}

// Report implements sim.FleetService.
func (f *observedFleet) Report(vehicleID, edgeID string, speedMPS float64, now time.Time) error {
	err := f.inner.Report(vehicleID, edgeID, speedMPS, now)

	f.mu.Lock()
	defer f.mu.Unlock()

	if err == nil {
		f.reports++
		return nil
	}

	f.reportFailures++
	if !f.loggedReport {
		f.loggedReport = true
		f.logger.Error("a simulated report failed",
			slog.String("vehicle", vehicleID),
			slog.String("edge", edgeID),
			slog.Float64("speed_mps", speedMPS),
			slog.String("error", err.Error()),
			slog.String("note", "the run continues and counts it; further report failures are counted, not logged"))
	}
	return err
}

// observedCustomFleet is the wrapper for a service that can also refresh weights.
// The customizer is kept as the original, so the calls go where they did before.
type observedCustomFleet struct {
	*observedFleet
	customizer sim.FleetCustomizer
}

// CustomizeEvery implements sim.FleetCustomizer.
func (f *observedCustomFleet) CustomizeEvery() time.Duration {
	return f.customizer.CustomizeEvery()
}

// DecayStale implements sim.FleetCustomizer.
func (f *observedCustomFleet) DecayStale(now time.Time) { f.customizer.DecayStale(now) }

// Customize implements sim.FleetCustomizer.
func (f *observedCustomFleet) Customize() *engine.Metric { return f.customizer.Customize() }

// traffic is what the run did across the wire: the calls that succeeded and the
// ones that did not.
type traffic struct {
	Routes         int
	Reports        int
	RouteFailures  int
	ReportFailures int
}

// traffic returns the counts so far.
func (f *observedFleet) traffic() traffic {
	f.mu.Lock()
	defer f.mu.Unlock()

	return traffic{
		Routes:         f.routes,
		Reports:        f.reports,
		RouteFailures:  f.routeFailures,
		ReportFailures: f.reportFailures,
	}
}
