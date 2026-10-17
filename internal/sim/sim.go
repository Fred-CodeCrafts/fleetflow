package sim

import (
	"errors"

	"fleetflow/internal/graph"
)

// Config configures a simulation run.
type Config struct {
	Seed       int64
	Vehicles   int
	FleetShare float64
	WindowS    float64
	StepS      float64
}

// DefaultConfig returns a demo-sized starting point.
func DefaultConfig() Config {
	return Config{
		Seed:       1,
		Vehicles:   200,
		FleetShare: 0.5,
		WindowS:    600,
		StepS:      1,
	}
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
	completed []float64
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
		g:   g,
		cfg: cfg,
		occ: NewOccupancy(len(g.Edges)),
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
func (s *Sim) CompletedTrips() []float64 { return s.completed }

// Occupancy returns the live per-edge vehicle counts. Callers that keep them
// should use Snapshot instead.
func (s *Sim) Occupancy() *Occupancy { return s.occ }

// Step advances the simulation by one time step.
func (s *Sim) Step() {
	dt := s.cfg.StepS
	s.nowS += dt
	s.advanceVehicles(dt)
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
			s.completed = append(s.completed, v.arriveS-v.departS)
			return
		}
		s.occ.Enter(v.route[v.leg])
	}
}

// placeOnRoute puts a vehicle on the network with the given edge route, starting
// at atS. An empty route means the vehicle is already at its destination.
func (s *Sim) placeOnRoute(v *Vehicle, route []int, atS float64) {
	if len(route) == 0 {
		v.state = VehicleArrived
		v.arriveS = atS
		s.completed = append(s.completed, atS-v.departS)
		return
	}

	v.route = route
	v.leg = 0
	v.travelledM = 0
	v.state = VehicleRunning
	s.occ.Enter(route[0])
}
