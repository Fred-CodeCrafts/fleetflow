package sim

// VehicleState tracks where a vehicle is in its trip.
type VehicleState uint8

const (
	// VehiclePending has not departed yet.
	VehiclePending VehicleState = iota
	// VehicleRunning is on the network.
	VehicleRunning
	// VehicleArrived has finished its trip.
	VehicleArrived
	// VehicleUnroutable could not be given a route, so it never enters the
	// network.
	VehicleUnroutable
)

// String implements fmt.Stringer.
func (s VehicleState) String() string {
	switch s {
	case VehiclePending:
		return "pending"
	case VehicleRunning:
		return "running"
	case VehicleArrived:
		return "arrived"
	case VehicleUnroutable:
		return "unroutable"
	default:
		return "unknown"
	}
}

// Vehicle is one simulated car driving its trip across the road graph. The
// simulator owns the fields; the accessors below are what the API and the
// dashboard read.
type Vehicle struct {
	demand Demand

	// route holds the edge indices from the current leg to the destination.
	route []int
	// leg is the position in route the vehicle is currently driving.
	leg int
	// travelledM is how far the vehicle has driven along the current edge.
	// Metres are tracked rather than a fraction so that repeated additions do
	// not drift and a vehicle does not overshoot the end of an edge.
	travelledM float64

	state        VehicleState
	departS      float64
	arriveS      float64
	lastRouteS   float64
	routeChanges int

	// freeFlowS accumulates the free-flow time of the edges this trip has
	// actually entered, which is the baseline the fairness check measures time
	// lost against.
	freeFlowS float64
}

// newVehicle creates a pending vehicle for the given trip.
func newVehicle(d Demand) *Vehicle {
	return &Vehicle{demand: d, departS: d.DepartS, state: VehiclePending}
}

// ID returns the vehicle's identifier.
func (v *Vehicle) ID() string { return v.demand.VehicleID }

// IsFleet reports whether the vehicle uses the routing service instead of a
// fixed free-flow route.
func (v *Vehicle) IsFleet() bool { return v.demand.Fleet }

// Origin returns the trip's start node.
func (v *Vehicle) Origin() string { return v.demand.From }

// Destination returns the trip's end node.
func (v *Vehicle) Destination() string { return v.demand.To }

// DepartureS returns the simulated time the vehicle is due to depart.
func (v *Vehicle) DepartureS() float64 { return v.departS }

// State reports whether the vehicle is pending, running or arrived.
func (v *Vehicle) State() VehicleState { return v.state }

// TravelledM reports how far the vehicle has driven along its current edge.
func (v *Vehicle) TravelledM() float64 { return v.travelledM }

// Progress reports the fraction of the current edge already covered, in [0, 1).
// edgeLengthM is the length of the edge the vehicle is on.
func (v *Vehicle) Progress(edgeLengthM float64) float64 {
	if edgeLengthM <= 0 {
		return 0
	}
	return v.travelledM / edgeLengthM
}

// RouteChanges reports how many times the vehicle switched to a different route.
func (v *Vehicle) RouteChanges() int { return v.routeChanges }

// TripDuration reports how long the trip took in simulated seconds. The second
// result is false until the vehicle has arrived.
func (v *Vehicle) TripDuration() (float64, bool) {
	if v.state != VehicleArrived {
		return 0, false
	}
	return v.arriveS - v.departS, true
}

// CurrentEdge returns the index in the graph's edge slice of the edge the
// vehicle is driving, or -1 when it is not on the network. The API needs it to
// place the vehicle: a vehicle is somewhere along an edge, and where that edge
// runs is the graph's business, not the simulator's.
func (v *Vehicle) CurrentEdge() int {
	if v.state != VehicleRunning || v.leg >= len(v.route) {
		return -1
	}
	return v.route[v.leg]
}
