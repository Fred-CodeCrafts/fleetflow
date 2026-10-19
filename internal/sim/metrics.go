package sim

import (
	"math"
	"sort"

	"fleetflow/internal/emissions"
)

// Metrics summarises a simulation run so that two runs on the same demand can be
// compared. Trip statistics cover the vehicles that have finished; the
// congestion figures are the state at the moment Collect was called.
type Metrics struct {
	TotalVehicles          int
	CompletedTrips         int
	AverageTripS           float64
	P95TripS               float64
	RouteChanges           int
	RouteChangesPerVehicle float64
	UnroutableVehicles     int
	// ReportsFailed is how many reports this arm's fleet service refused. One
	// dropped report only makes the weights slightly stale; a run of them means
	// the arm is not being told about its own traffic at all. On the HTTP path
	// that is what a stopped server or a rejected key looks like, and without
	// this figure it would be read as a run with nothing to report.
	ReportsFailed int

	SaturatedEdges    int
	SaturatedEdgesPct float64
	UtilizationStdDev float64

	// Fairness is the US-6 fairness check. It is reported beside the headline
	// figures above, not as one of them: none of it says that FleetFlow wins.
	Fairness Fairness

	// Emissions is the estimated exhaust output of the trips that finished, and
	// DistanceKM is how far they drove in total. Both cover the same population
	// as the trip statistics, so an emissions figure can be read against the
	// average trip time without a second denominator.
	Emissions  Emissions
	DistanceKM float64
}

// Emissions is a run arm's estimated exhaust output, in grams, over the trips
// that finished.
//
// It is computed from each trip's own distance and mean speed, using the same
// model for both arms, so the comparison between them is like for like. The
// figures are estimates under a documented model rather than a measurement; see
// internal/emissions for what it does and does not account for.
type Emissions struct {
	CO2G       float64
	NOXG       float64
	COG        float64
	HCG        float64
	PMG        float64
	FuelLitres float64
}

// Fairness holds the fairness check that sits next to the headline metrics but
// is not a headline result. It records what the trip times cost against free
// flow and how the two populations of vehicles were treated, so a result can be
// read honestly either way rather than only when the fleet looks good.
type Fairness struct {
	// MeanTimeLostS is the mean, over completed trips, of the actual trip
	// duration minus the free-flow duration of the route the vehicle actually
	// drove, in simulated seconds. Time lost is what congestion and detours cost
	// the vehicle; the baseline follows the driven route, not a theoretical best
	// one, so a detour is not scored as free-flow time.
	MeanTimeLostS float64
	// TotalTimeLostS is the sum of those per-trip differences, in simulated
	// seconds.
	TotalTimeLostS float64

	// FleetTrips is how many completed trips were driven by fleet vehicles, and
	// MeanFleetTripS is their mean duration in simulated seconds. Zero when no
	// fleet trip has finished.
	FleetTrips     int
	MeanFleetTripS float64
	// BackgroundTrips and MeanBackgroundTripS are the same for background
	// vehicles, which never use the routing service.
	BackgroundTrips     int
	MeanBackgroundTripS float64
}

// Collect computes the metrics of the run at its current state.
func (s *Sim) Collect() Metrics {
	m := Metrics{TotalVehicles: len(s.vehicles)}

	trips := make([]float64, 0, len(s.completed))
	var timeLostTotal float64
	var fleetSum, backgroundSum float64
	var totalDistanceM float64
	for _, t := range s.completed {
		trips = append(trips, t.durationS)
		timeLostTotal += t.durationS - t.freeFlowS
		totalDistanceM += t.distanceM

		// Each trip is estimated on its own distance and mean speed, then summed.
		// Estimating the whole run from its mean speed instead would be a
		// different, and wrong, calculation: a run with one crawling trip and one
		// fast trip does not emit what a single trip at the average speed emits,
		// because the emission curve is not linear in speed.
		if t.distanceM > 0 && t.durationS > 0 {
			meanKmh := (t.distanceM / 1000) / (t.durationS / 3600)
			if est, err := emissions.Trip(emissions.PassengerCar, t.distanceM/1000, meanKmh); err == nil {
				m.Emissions.CO2G += est.CO2G
				m.Emissions.NOXG += est.NOXG
				m.Emissions.COG += est.COG
				m.Emissions.HCG += est.HCG
				m.Emissions.PMG += est.PMG
				m.Emissions.FuelLitres += est.FuelLitres
			}
		}

		if t.fleet {
			m.Fairness.FleetTrips++
			fleetSum += t.durationS
		} else {
			m.Fairness.BackgroundTrips++
			backgroundSum += t.durationS
		}
	}
	m.DistanceKM = totalDistanceM / 1000
	sort.Float64s(trips)
	m.CompletedTrips = len(trips)
	if len(trips) > 0 {
		var sum float64
		for _, t := range trips {
			sum += t
		}
		m.AverageTripS = sum / float64(len(trips))
		m.P95TripS = percentile(trips, 95)
		m.Fairness.MeanTimeLostS = timeLostTotal / float64(len(trips))
		m.Fairness.TotalTimeLostS = timeLostTotal
	}
	if m.Fairness.FleetTrips > 0 {
		m.Fairness.MeanFleetTripS = fleetSum / float64(m.Fairness.FleetTrips)
	}
	if m.Fairness.BackgroundTrips > 0 {
		m.Fairness.MeanBackgroundTripS = backgroundSum / float64(m.Fairness.BackgroundTrips)
	}

	for _, v := range s.vehicles {
		m.RouteChanges += v.routeChanges
		if v.state == VehicleUnroutable {
			m.UnroutableVehicles++
		}
	}
	m.ReportsFailed = s.reportFailures
	if len(s.vehicles) > 0 {
		m.RouteChangesPerVehicle = float64(m.RouteChanges) / float64(len(s.vehicles))
	}

	utilizations := make([]float64, 0, len(s.g.Edges))
	for i, e := range s.g.Edges {
		if e.Capacity <= 0 {
			continue
		}
		count := s.occ.Count(i)
		utilizations = append(utilizations, float64(count)/e.Capacity)
		// An edge is saturated when it carries at least its capacity in vehicles.
		if float64(count) >= e.Capacity {
			m.SaturatedEdges++
		}
	}
	if len(s.g.Edges) > 0 {
		m.SaturatedEdgesPct = float64(m.SaturatedEdges) / float64(len(s.g.Edges)) * 100
	}
	if len(utilizations) > 0 {
		var sum float64
		for _, u := range utilizations {
			sum += u
		}
		m.UtilizationStdDev = stdDev(utilizations, sum/float64(len(utilizations)))
	}

	return m
}

// percentile returns the p-th percentile of an ascending slice using the
// nearest-rank method. p is a percentage in [0, 100].
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

// stdDev returns the population standard deviation of values around mean.
func stdDev(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sumSquares float64
	for _, v := range values {
		d := v - mean
		sumSquares += d * d
	}
	return math.Sqrt(sumSquares / float64(len(values)))
}
