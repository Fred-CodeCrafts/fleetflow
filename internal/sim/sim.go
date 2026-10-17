package sim

import (
	"errors"
	"time"

	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
)

// FleetService is the routing service that fleet vehicles talk to: they ask it
// for routes, price the routes they already hold, and report the speeds they
// observe. *engine.Engine satisfies it.
type FleetService interface {
	Route(fromID, toID string) (*engine.Path, error)
	PathCost(edges []int) (float64, bool)
	Report(edgeID string, speedMPS float64, now time.Time) error
}

// FleetCustomizer is implemented by a fleet service whose weights are refreshed
// on the simulation clock rather than a wall clock. *engine.Engine satisfies it.
type FleetCustomizer interface {
	// CustomizeEvery reports how much simulated time should pass between
	// refreshes.
	CustomizeEvery() time.Duration
	// DecayStale relaxes edges that have gone quiet, at the given time.
	DecayStale(now time.Time)
	// Customize rebuilds and publishes the weights.
	Customize() *engine.Metric
}

// reportEpoch anchors simulated seconds to a real timestamp for reports, so runs
// stay deterministic instead of depending on the wall clock.
var reportEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// Config configures a simulation run.
type Config struct {
	Seed       int64
	Vehicles   int
	FleetShare float64
	WindowS    float64
	StepS      float64
	// RerouteEveryS is how often, in simulated seconds, a running fleet vehicle
	// reconsiders its route. Zero disables rerouting.
	RerouteEveryS float64
	// RerouteThreshold is the fractional cost improvement a new route must beat
	// the current one by before the vehicle switches. It is the hysteresis that
	// stops vehicles flapping between two similar routes.
	RerouteThreshold float64
}

// DefaultConfig returns a demo-sized starting point.
func DefaultConfig() Config {
	return Config{
		Seed:             1,
		Vehicles:         200,
		FleetShare:       0.5,
		WindowS:          600,
		StepS:            1,
		RerouteEveryS:    30,
		RerouteThreshold: 0.10,
	}
}

// completedTrip is one finished trip as the fairness check needs it: how long it
// took, what free flow would have cost on the route actually driven, and which
// population the vehicle belonged to.
type completedTrip struct {
	durationS float64
	freeFlowS float64
	fleet     bool
}

// Sim advances vehicles over the road graph. It is not safe for concurrent use;
// the API layer drives it from a single goroutine and publishes snapshots for
// readers.
type Sim struct {
	g        *graph.Graph
	cfg      Config
	occ      *Occupancy
	vehicles []*Vehicle

	nowS      float64
	completed []completedTrip

	// Background vehicles never contact an engine, so they route on a fixed
	// free-flow metric of their own.
	staticMetric *engine.Metric
	staticRouter engine.Router

	// fleet is the routing service fleet vehicles use. Without one they fall
	// back to static routes.
	fleet FleetService

	// customizer is set when the fleet service refreshes its weights on the
	// simulation clock, so the simulation can drive that refresh.
	customizer     FleetCustomizer
	lastCustomizeS float64
}

// New builds a simulation whose vehicles start pending. They step onto the
// network when their scheduled departure time arrives.
func New(g *graph.Graph, cfg Config) (*Sim, error) {
	if g == nil {
		return nil, errors.New("graph is required")
	}
	if cfg.StepS <= 0 {
		return nil, errors.New("step size must be positive")
	}
	if cfg.Vehicles < 0 {
		return nil, errors.New("vehicle count must not be negative")
	}

	demand := GenerateDemand(g, cfg.Seed, DemandConfig{
		Vehicles:   cfg.Vehicles,
		FleetShare: cfg.FleetShare,
		WindowS:    cfg.WindowS,
	})

	s := &Sim{
		g:            g,
		cfg:          cfg,
		occ:          NewOccupancy(len(g.Edges)),
		staticMetric: engine.NewStaticMetric(g),
		staticRouter: engine.NewDijkstraRouter(g),
	}
	s.vehicles = make([]*Vehicle, len(demand))
	for i, d := range demand {
		s.vehicles[i] = newVehicle(d)
	}
	return s, nil
}

// Config returns the configuration the simulation was created with.
func (s *Sim) Config() Config { return s.cfg }

// NowS returns the simulated time in seconds.
func (s *Sim) NowS() float64 { return s.nowS }

// Vehicles returns the simulation's vehicles.
func (s *Sim) Vehicles() []*Vehicle { return s.vehicles }

// CompletedTrips returns the durations of the trips finished so far, in
// simulated seconds.
func (s *Sim) CompletedTrips() []float64 {
	durations := make([]float64, len(s.completed))
	for i, t := range s.completed {
		durations[i] = t.durationS
	}
	return durations
}

// Occupancy returns the live per-edge vehicle counts. Callers that keep them
// should use Snapshot instead.
func (s *Sim) Occupancy() *Occupancy { return s.occ }

// Step advances the simulation by one time step: vehicles whose departure time
// has arrived enter the network, weights are refreshed if a customization
// interval has passed, everyone moves, and fleet vehicles report the speeds they
// observed.
func (s *Sim) Step() {
	dt := s.cfg.StepS
	s.nowS += dt
	s.departDue()
	s.refreshWeights()
	s.advanceVehicles(dt)
	s.rerouteFleet()
	s.reportSpeeds()
}

// SetFleetService attaches the routing service that fleet vehicles use. Without
// one, fleet vehicles fall back to the same static routes as background traffic.
// A service that also implements FleetCustomizer has its weights refreshed on
// the simulation clock.
func (s *Sim) SetFleetService(f FleetService) {
	s.fleet = f
	s.customizer = nil
	if c, ok := f.(FleetCustomizer); ok {
		s.customizer = c
	}
}

// refreshWeights relaxes quiet edges and republishes the weights once a
// customization interval of simulated time has passed. Running it here, rather
// than on a wall clock, keeps a run deterministic and makes the weights age with
// simulated time.
func (s *Sim) refreshWeights() {
	if s.customizer == nil {
		return
	}

	everyS := s.customizer.CustomizeEvery().Seconds()
	if everyS <= 0 || s.nowS-s.lastCustomizeS < everyS {
		return
	}

	s.customizer.DecayStale(s.ReportTime())
	s.customizer.Customize()
	s.lastCustomizeS = s.nowS
}

// ReportTime returns the timestamp used for the reports of the current step.
func (s *Sim) ReportTime() time.Time {
	return reportEpoch.Add(time.Duration(s.nowS * float64(time.Second)))
}

// departDue starts every pending vehicle whose scheduled departure has arrived.
func (s *Sim) departDue() {
	for _, v := range s.vehicles {
		if v.state == VehiclePending && v.departS <= s.nowS {
			s.depart(v)
		}
	}
}

// depart puts a pending vehicle on the network. The trip clock starts when the
// vehicle actually enters, which is the scheduled time on an uncongested run.
func (s *Sim) depart(v *Vehicle) {
	path, err := s.routeFor(v)
	if err != nil || path == nil {
		v.state = VehicleUnroutable
		return
	}
	v.departS = s.nowS
	v.lastRouteS = s.nowS
	s.placeOnRoute(v, path.Edges, s.nowS)
}

// routeFor returns the edge route a departing vehicle should follow. Fleet
// vehicles ask the routing service; background vehicles use the fixed free-flow
// route they would drive without any coordination.
func (s *Sim) routeFor(v *Vehicle) (*engine.Path, error) {
	if v.IsFleet() && s.fleet != nil {
		return s.fleet.Route(v.Origin(), v.Destination())
	}
	return s.staticRouter.Route(s.staticMetric, v.Origin(), v.Destination())
}

// rerouteFleet lets running fleet vehicles reconsider their route. A vehicle
// only switches when the new route is cheaper than the rest of the current one
// by more than the hysteresis threshold, which limits flapping.
//
// The current edge is never given up: the vehicle must finish it, so only the
// edges after it are compared.
func (s *Sim) rerouteFleet() {
	if s.fleet == nil || s.cfg.RerouteEveryS <= 0 {
		return
	}

	for _, v := range s.vehicles {
		if !v.IsFleet() || v.state != VehicleRunning {
			continue
		}
		if s.nowS-v.lastRouteS < s.cfg.RerouteEveryS {
			continue
		}
		s.reroute(v)
	}
}

// reroute reconsiders one vehicle's remaining route.
func (s *Sim) reroute(v *Vehicle) {
	edgeIndex := v.currentEdge()
	if edgeIndex < 0 {
		return
	}

	tail := v.route[v.leg+1:]
	currentCost, ok := s.fleet.PathCost(tail)
	if !ok {
		return
	}

	// The candidate starts at the node the vehicle is heading towards.
	candidate, err := s.fleet.Route(s.g.Edges[edgeIndex].To, v.Destination())
	if err != nil || candidate == nil {
		return
	}

	if candidate.CostS >= currentCost*(1-s.cfg.RerouteThreshold) {
		return // not enough of an improvement to be worth switching
	}

	newRoute := make([]int, 0, len(candidate.Edges)+1)
	newRoute = append(newRoute, edgeIndex)
	newRoute = append(newRoute, candidate.Edges...)

	// The vehicle stays on the same edge and at the same spot, so occupancy is
	// unchanged; only the edges ahead are replaced.
	v.route = newRoute
	v.leg = 0
	v.routeChanges++
	v.lastRouteS = s.nowS
}

// reportSpeeds lets every running fleet vehicle tell the service how fast it is
// going on the edge it occupies. Report failures are ignored: a dropped report
// only makes the weights slightly stale.
func (s *Sim) reportSpeeds() {
	if s.fleet == nil {
		return
	}

	now := s.ReportTime()
	for _, v := range s.vehicles {
		if !v.IsFleet() || v.state != VehicleRunning {
			continue
		}
		edgeIndex := v.currentEdge()
		if edgeIndex < 0 {
			continue
		}
		edge := s.g.Edges[edgeIndex]
		speed := EdgeSpeed(edge.SpeedLimitMPS, edge.Capacity, s.occ.Count(edgeIndex))
		_ = s.fleet.Report(edge.ID, speed, now)
	}
}

// advanceVehicles moves every running vehicle along its route and finishes the
// trips that reach their destination.
func (s *Sim) advanceVehicles(dt float64) {
	for _, v := range s.vehicles {
		if v.state == VehicleRunning {
			s.advanceVehicle(v, dt)
		}
	}
}

// advanceVehicle moves one vehicle by up to dt seconds of travel. It walks as
// many edges as the time allows, so a long step or a short edge cannot make it
// lose time.
func (s *Sim) advanceVehicle(v *Vehicle, dt float64) {
	remaining := dt

	for remaining > 0 && v.state == VehicleRunning {
		edgeIndex := v.route[v.leg]
		edge := s.g.Edges[edgeIndex]

		speed := EdgeSpeed(edge.SpeedLimitMPS, edge.Capacity, s.occ.Count(edgeIndex))
		if speed <= 0 {
			return // the edge is closed to traffic; the vehicle waits
		}

		distanceLeft := edge.LengthM - v.travelledM
		travel := speed * remaining
		if travel < distanceLeft {
			v.travelledM += travel
			return
		}

		// This edge is done; spend the leftover time on the next one.
		remaining -= distanceLeft / speed
		s.occ.Leave(edgeIndex)
		v.leg++
		v.travelledM = 0

		if v.leg >= len(v.route) {
			v.state = VehicleArrived
			v.arriveS = s.nowS - remaining
			s.completeTrip(v)
			return
		}
		s.enterEdge(v, v.route[v.leg])
	}
}

// enterEdge drives a vehicle onto an edge. It charges the edge's free-flow time
// against the trip as the edge is entered, so the fairness baseline is the route
// the vehicle actually drove: edges dropped by a reroute are never charged, and
// an edge taken twice is charged twice.
func (s *Sim) enterEdge(v *Vehicle, edgeIndex int) {
	v.freeFlowS += s.g.Edges[edgeIndex].FreeFlowS
	s.occ.Enter(edgeIndex)
}

// completeTrip records an arrived vehicle's trip, together with the free-flow
// time accumulated over the route it actually drove.
func (s *Sim) completeTrip(v *Vehicle) {
	s.completed = append(s.completed, completedTrip{
		durationS: v.arriveS - v.departS,
		freeFlowS: v.freeFlowS,
		fleet:     v.IsFleet(),
	})
}

// placeOnRoute puts a vehicle on the network with the given edge route, starting
// at atS. An empty route means the vehicle is already at its destination.
func (s *Sim) placeOnRoute(v *Vehicle, route []int, atS float64) {
	v.freeFlowS = 0 // each placement is a fresh trip

	if len(route) == 0 {
		v.state = VehicleArrived
		v.arriveS = atS
		s.completeTrip(v)
		return
	}

	v.route = route
	v.leg = 0
	v.travelledM = 0
	v.state = VehicleRunning
	s.enterEdge(v, route[0])
}
