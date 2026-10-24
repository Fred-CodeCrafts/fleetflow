package sim

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
)

// FleetService is the routing service that fleet vehicles talk to: they ask it
// to plan a route — and, when they already hold one, what that route costs — and
// they report the speeds they observe.
//
// Planning and pricing come back from one call so the two are always measured on
// the same snapshot, whether the service is the engine in-process or the public
// HTTP API. That is the seam DESIGN 3.4 describes: tests inject a fake or the
// engine, and production points an HTTP client at the server's own base URL.
type FleetService interface {
	// Route plans a path from fromID to toID. When held is non-nil the answer
	// also prices those edge indices on the same snapshot as the plan.
	Route(fromID, toID string, held []int) (engine.RouteAnswer, error)
	// Report publishes one speed observation a vehicle made on an edge. The
	// vehicle is named because the public API requires it, and it is what lets a
	// reader tell a simulated report from a live one.
	Report(vehicleID, edgeID string, speedMPS float64, now time.Time) error
}

// BatchReporter is a FleetService that can publish a whole step's observations in
// one call. A service that does not implement it still works: the simulator falls
// back to Report per vehicle.
//
// It exists because congestion is a property of an edge, not of a vehicle. Every
// vehicle on an edge observes the same speed, so a step that moves 60,000
// vehicles produces a few thousand distinct observations, not 30,000 — and over
// HTTP, one request instead of 30,000. Measured on the Jakarta network, per-vehicle
// reporting over the public API advanced the run two simulated seconds in forty
// seconds of wall clock; batching is what makes a large run watchable at all.
type BatchReporter interface {
	// ReportBatch publishes observations made at one simulated instant. now is
	// the instant, and obs is read-only for the duration of the call.
	ReportBatch(obs []SpeedObservation, now time.Time) error
}

// SpeedObservation is one edge's observed speed at one instant. The vehicle is
// the one that happened to be sampled for the edge, and is what the API records
// so a reader can tell a simulated report from a live one.
type SpeedObservation struct {
	VehicleID string
	EdgeID    string
	SpeedMPS  float64
}

// ConcurrentRouter is a FleetService that may be called from several goroutines at
// once.
//
// A route query reads one immutable metric snapshot, so concurrency is safe in the
// engine and in the HTTP client. Declaring it separately matters because the
// simulator must not assume it: a test fake that counts calls, or a service that
// keeps a mutable cursor, would race if the simulator planned a step's departures
// in parallel without being told it could.
type ConcurrentRouter interface {
	// RouteConcurrently plans a path from fromID to toID. It must be safe to call
	// from multiple goroutines and must not depend on the order of calls.
	RouteConcurrently(fromID, toID string) (*engine.Path, error)
}

// EngineFleet serves a routing engine to the simulator in-process. The comparison
// runs and the unit tests use it; production uses the HTTP client instead, so
// simulated traffic takes the public, validated path.
//
// It also satisfies FleetCustomizer, so a simulation driven by an engine keeps
// refreshing that engine's weights on the simulation clock.
type EngineFleet struct{ eng *engine.Engine }

// NewEngineFleet adapts an engine to FleetService.
func NewEngineFleet(e *engine.Engine) *EngineFleet { return &EngineFleet{eng: e} }

// Route implements FleetService.
func (f *EngineFleet) Route(fromID, toID string, held []int) (engine.RouteAnswer, error) {
	return f.eng.RouteWithHeld(fromID, toID, held)
}

// Report implements FleetService. The engine keeps no per-vehicle state, so the
// vehicle a report names is only the API's concern; in-process it is not read.
func (f *EngineFleet) Report(vehicleID, edgeID string, speedMPS float64, now time.Time) error {
	return f.eng.Report(edgeID, speedMPS, now)
}

// CustomizeEvery implements FleetCustomizer.
func (f *EngineFleet) CustomizeEvery() time.Duration { return f.eng.CustomizeEvery() }

// DecayStale implements FleetCustomizer.
func (f *EngineFleet) DecayStale(now time.Time) { f.eng.DecayStale(now) }

// Customize implements FleetCustomizer.
func (f *EngineFleet) Customize() *engine.Metric { return f.eng.Customize() }

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

// DefaultConfig returns the demo scenario: a peak, not a trickle.
//
// The window matters more than the vehicle count. What congests a road is how
// many trips are on it at once, and with a wide departure window the same fleet
// spreads out so thinly that no edge ever approaches capacity -- at which point
// static and dynamic routing make identical choices and the comparison says
// nothing. Concentrating the departures into a short window is what a morning
// peak does, and it is what gives the dynamic router something to do.
//
// The numbers are measured, not chosen for effect: see DESIGN, "The comparison
// result", for what this configuration produces and for what the uniform
// version of it does not.
func DefaultConfig() Config {
	return Config{
		Seed:             1,
		Vehicles:         2000,
		FleetShare:       0.5,
		WindowS:          30,
		StepS:            1,
		RerouteEveryS:    30,
		RerouteThreshold: 0.10,
	}
}

// completedTrip is one finished trip as the fairness check and the emissions
// estimate need it: how long it took, what free flow would have cost on the
// route actually driven, how far that route was, and which population the
// vehicle belonged to.
type completedTrip struct {
	durationS float64
	freeFlowS float64
	// distanceM is the length of the route actually driven, accumulated edge by
	// edge. Emissions are a function of distance and mean speed, and a detour
	// caused by congestion must count at its real length.
	distanceM float64
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

	// reportFailures counts reports the fleet service refused. A single dropped
	// report costs one observation and the run carries on, but a run whose arm
	// can never report has no dynamic weights at all, and without a count it
	// looks exactly like a run with nothing to report.
	reportFailures int

	// batchScratch and reportedEdgeStamp support reporting one observation per
	// edge per step. batchScratch is reused between steps so a step allocates
	// nothing; reportedEdgeStamp marks which edges this step has already covered,
	// carrying a generation in reportStep rather than being cleared, because
	// clearing it is a pass over every edge in the graph.
	batchScratch      []SpeedObservation
	reportedEdgeStamp []uint32
	reportStep        uint32
	dueScratch        []*Vehicle
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
		g:                 g,
		cfg:               cfg,
		occ:               NewOccupancy(len(g.Edges)),
		staticMetric:      engine.NewStaticMetric(g),
		staticRouter:      engine.NewDijkstraRouter(g),
		reportedEdgeStamp: make([]uint32, len(g.Edges)),
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

	// Vehicles due to leave are placed at this step's clock time and then move
	// over the step that follows it, so a trip's elapsed time covers exactly the
	// travel it was charged for. Departing after the clock had already moved gave
	// every vehicle a step of travel its departure stamp did not cover, and every
	// trip came out one step shorter than the free-flow time of the route it drove
	// -- which is the number the fairness check reads as time lost (T34a).
	s.departDue()
	s.nowS += dt

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
	due := s.dueScratch[:0]
	for _, v := range s.vehicles {
		if v.state == VehiclePending && v.departS <= s.nowS {
			due = append(due, v)
		}
	}
	s.dueScratch = due

	// Route the departures concurrently when the fleet service allows it. A
	// departure's only work is a route query, and over the public API each one is
	// a round trip: a 60,000-vehicle run over a 30 s window departs 2,000 vehicles
	// in a step, which is 2,000 round trips end to end. The queries are independent
	// and the engine answers them from one immutable snapshot, so the only thing
	// ordering them achieves is waiting.
	//
	// A service that cannot route concurrently keeps the sequential path, which is
	// what the in-process engine and the tests use.
	if concurrent, ok := s.fleet.(ConcurrentRouter); ok && len(due) > 1 {
		s.routeDeparturesConcurrently(due, concurrent)
		return
	}

	for _, v := range due {
		s.depart(v)
	}
}

// maxConcurrentRoutes bounds how many route queries a step has in flight. The
// bound is on simultaneous round trips, not on vehicles: past a few dozen the
// client is queueing at the server rather than saving wall time, and the server
// has other work to do.
const maxConcurrentRoutes = 32

// routeDeparturesConcurrently plans a step's departures with several queries in
// flight, then places each vehicle on its route in the original order.
//
// Placement stays sequential because it touches shared state — occupancy, the
// completed-trip log — while planning does not: it reads one published snapshot.
// That is the same split the engine is built around.
func (s *Sim) routeDeparturesConcurrently(due []*Vehicle, router ConcurrentRouter) {
	type planned struct {
		v    *Vehicle
		path *engine.Path
		err  error
	}

	results := make([]planned, len(due))
	var next atomic.Int64

	workers := min(maxConcurrentRoutes, len(due))
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(due) {
					return
				}
				v := due[i]
				path, err := router.RouteConcurrently(v.Origin(), v.Destination())
				results[i] = planned{v: v, path: path, err: err}
			}
		}()
	}
	wg.Wait()

	for _, r := range results {
		if r.err != nil || r.path == nil {
			r.v.state = VehicleUnroutable
			continue
		}
		r.v.departS = s.nowS
		r.v.lastRouteS = s.nowS
		s.placeOnRoute(r.v, r.path.Edges, s.nowS)
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
		answer, err := s.fleet.Route(v.Origin(), v.Destination(), nil)
		if err != nil {
			return nil, err
		}
		return answer.Path, nil
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
//
// The plan and the price of the tail come back from one call, so both are measured
// on the same snapshot and the comparison means what it says. Pricing them with
// two calls could straddle a customization and read one cost against the other.
func (s *Sim) reroute(v *Vehicle) {
	// The check happened, whatever it decided. Recording it only on a successful
	// switch made a declined vehicle re-query on every single step: on the
	// Jakarta network that was 1,406 route queries for 50 vehicles over 100
	// simulated seconds, against 24 for the same vehicles before they started
	// declining. The interval is how often a vehicle may ask, not how often it
	// may change its mind.
	v.lastRouteS = s.nowS

	edgeIndex := v.CurrentEdge()
	if edgeIndex < 0 {
		return
	}

	// The vehicle must finish the edge it is on, so only the edges after it are
	// at stake. With nothing left there is nothing to switch to.
	tail := v.route[v.leg+1:]
	if len(tail) == 0 {
		return
	}

	// The candidate starts at the node the vehicle is heading towards, and the
	// tail is priced alongside it.
	answer, err := s.fleet.Route(s.g.Edges[edgeIndex].To, v.Destination(), tail)
	if err != nil || answer.Path == nil || !answer.HeldPriced {
		return
	}

	if answer.Path.CostS >= answer.HeldCostS*(1-s.cfg.RerouteThreshold) {
		return // not enough of an improvement to be worth switching
	}

	newRoute := make([]int, 0, len(answer.Path.Edges)+1)
	newRoute = append(newRoute, edgeIndex)
	newRoute = append(newRoute, answer.Path.Edges...)

	// The vehicle stays on the same edge and at the same spot, so occupancy is
	// unchanged; only the edges ahead are replaced.
	v.route = newRoute
	v.leg = 0
	v.routeChanges++
}

// reportSpeeds lets every running fleet vehicle tell the service how fast it is
// going on the edge it occupies. A refused report does not stop the run -- the
// weights only go stale -- but it is counted, because "one report was dropped"
// and "this arm cannot report at all" are otherwise the same picture.
func (s *Sim) reportSpeeds() {
	if s.fleet == nil {
		return
	}

	now := s.ReportTime()

	// Congestion is a property of an edge, and every vehicle on an edge observes
	// the same speed, so one observation per edge is the whole information
	// content of a step. Reporting per vehicle instead repeated the same number
	// once per vehicle: on the Jakarta network a 60,000-vehicle step had 30,000
	// fleet vehicles on a few thousand distinct edges.
	//
	// A service that can take a batch gets one call per step. One that cannot
	// still gets one Report per vehicle, which is what the interface promises;
	// the deduplication only saves the network, not the engine.
	batch, canBatch := s.fleet.(BatchReporter)
	if !canBatch {
		for _, v := range s.vehicles {
			if !v.IsFleet() || v.state != VehicleRunning {
				continue
			}
			edgeIndex := v.CurrentEdge()
			if edgeIndex < 0 {
				continue
			}
			edge := s.g.Edges[edgeIndex]
			speed := EdgeSpeed(edge.SpeedLimitMPS, edge.Capacity, s.occ.Count(edgeIndex))
			if err := s.fleet.Report(v.ID(), edge.ID, speed, now); err != nil {
				s.reportFailures++
			}
		}
		return
	}

	s.batchScratch = s.batchScratch[:0]
	// Bump the generation before the scan: the stamps start zeroed, so a step
	// that reused 0 would find every edge already marked.
	s.reportStep++
	for _, v := range s.vehicles {
		if !v.IsFleet() || v.state != VehicleRunning {
			continue
		}
		edgeIndex := v.CurrentEdge()
		if edgeIndex < 0 {
			continue
		}
		if s.reportedEdgeStamp[edgeIndex] == s.reportStep {
			continue // an earlier vehicle on this edge already reported it
		}
		s.reportedEdgeStamp[edgeIndex] = s.reportStep

		edge := s.g.Edges[edgeIndex]
		speed := EdgeSpeed(edge.SpeedLimitMPS, edge.Capacity, s.occ.Count(edgeIndex))
		s.batchScratch = append(s.batchScratch, SpeedObservation{
			VehicleID: v.ID(),
			EdgeID:    edge.ID,
			SpeedMPS:  speed,
		})
	}

	if len(s.batchScratch) == 0 {
		return
	}
	if err := batch.ReportBatch(s.batchScratch, now); err != nil {
		s.reportFailures++
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
//
// It also charges the edge's length, which is what makes the trip's distance the
// distance it really covered rather than the distance between its endpoints. A
// reroute that detours must show the longer distance, or the emissions figure
// derived from it would understate the detour.
func (s *Sim) enterEdge(v *Vehicle, edgeIndex int) {
	edge := s.g.Edges[edgeIndex]
	v.freeFlowS += edge.FreeFlowS
	v.distanceM += edge.LengthM
	s.occ.Enter(edgeIndex)
}

// completeTrip records an arrived vehicle's trip, together with the free-flow
// time accumulated over the route it actually drove.
func (s *Sim) completeTrip(v *Vehicle) {
	s.completed = append(s.completed, completedTrip{
		durationS: v.arriveS - v.departS,
		freeFlowS: v.freeFlowS,
		distanceM: v.distanceM,
		fleet:     v.IsFleet(),
	})
}

// placeOnRoute puts a vehicle on the network with the given edge route, starting
// at atS. An empty route means the vehicle is already at its destination.
func (s *Sim) placeOnRoute(v *Vehicle, route []int, atS float64) {
	v.freeFlowS = 0 // each placement is a fresh trip
	v.distanceM = 0

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
