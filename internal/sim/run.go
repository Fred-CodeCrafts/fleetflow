package sim

import (
	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
)

// Comparison holds the metrics of two runs over identical demand: one where the
// fleet routes on static free-flow costs, and one where it routes on the dynamic
// weights.
type Comparison struct {
	Baseline  Metrics
	FleetFlow Metrics
}

// RunToCompletion steps the simulation until every vehicle has arrived or been
// dropped, or until maxSteps steps have passed. It returns the collected
// metrics. The step budget stops a run where some vehicles can never finish.
func (s *Sim) RunToCompletion(maxSteps int) Metrics {
	for i := 0; i < maxSteps; i++ {
		s.Step()
		if s.allSettled() {
			break
		}
	}
	return s.Collect()
}

// allSettled reports whether every vehicle has finished, one way or another.
func (s *Sim) allSettled() bool {
	for _, v := range s.vehicles {
		if v.state == VehiclePending || v.state == VehicleRunning {
			return false
		}
	}
	return true
}

// RunBoth runs the same demand twice and returns the metrics of each run: first
// with a baseline engine, then with a dynamic one. Both simulations are built
// from the same config and seed, so the demand is identical and the only
// difference is how the fleet routes.
func RunBoth(g *graph.Graph, cfg Config, params weights.Params, maxSteps int) (Comparison, error) {
	var comparison Comparison

	baseline, err := runOne(g, cfg, func() (FleetService, error) {
		return engine.NewBaseline(g, engine.NewDijkstraRouter(g))
	}, maxSteps)
	if err != nil {
		return Comparison{}, err
	}
	comparison.Baseline = baseline

	dynamic, err := runOne(g, cfg, func() (FleetService, error) {
		return engine.New(g, engine.NewDijkstraRouter(g), params)
	}, maxSteps)
	if err != nil {
		return Comparison{}, err
	}
	comparison.FleetFlow = dynamic

	return comparison, nil
}

// runOne builds a simulation from cfg, attaches a freshly built service and runs
// it to completion.
func runOne(g *graph.Graph, cfg Config, build func() (FleetService, error), maxSteps int) (Metrics, error) {
	service, err := build()
	if err != nil {
		return Metrics{}, err
	}

	s, err := New(g, cfg)
	if err != nil {
		return Metrics{}, err
	}
	s.SetFleetService(service)

	return s.RunToCompletion(maxSteps), nil
}
