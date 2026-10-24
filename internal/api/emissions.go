package api

import (
	"math"
	"net/http"
	"sort"
	"time"

	"fleetflow/internal/simulation"
	"fleetflow/internal/simulation/sumo"
)

// EmissionsSummary is one fleet-wide answer to "how much did we emit?". The
// console's emissions screens read it instead of walking every simulation run
// and its trips in the browser, so the numbers they show and the numbers any
// later exporter produces come from the same arithmetic.
type EmissionsSummary struct {
	Totals   EmissionsTotals    `json:"totals"`
	Runs     []EmissionsRun     `json:"runs"`
	Vehicles []EmissionsVehicle `json:"vehicles"`
	// Method is the emission provenance of the most recent contributing run,
	// passed through unchanged. It is absent when no run contributed, because
	// then there is no model to name.
	Method *sumo.EmissionProvenance `json:"method,omitempty"`
}

// EmissionsTotals is the fleet-wide roll-up over every contributing run.
type EmissionsTotals struct {
	Trips    int     `json:"trips"`
	Distance float64 `json:"distance_km"`
	CO2      float64 `json:"co2_kg"`
	Fuel     float64 `json:"fuel_kg"`
	CO2Rate  float64 `json:"g_co2_per_km"`
	Runs     int     `json:"runs"`
}

// EmissionsRun is one simulation run's contribution. Only completed runs with a
// result appear here.
type EmissionsRun struct {
	RunID     string  `json:"run_id"`
	CreatedAt string  `json:"created_at"`
	Vehicles  int     `json:"vehicles"`
	Trips     int     `json:"trips"`
	Distance  float64 `json:"distance_km"`
	CO2       float64 `json:"co2_kg"`
	Fuel      float64 `json:"fuel_kg"`
	CO2Rate   float64 `json:"g_co2_per_km"`
}

// EmissionsVehicle is one fleet vehicle's contribution across every
// contributing run. Trips SUMO could not attribute to a vehicle are counted in
// the totals but have no row here.
type EmissionsVehicle struct {
	VehicleID string  `json:"vehicle_id"`
	Trips     int     `json:"trips"`
	Distance  float64 `json:"distance_km"`
	CO2       float64 `json:"co2_kg"`
	Fuel      float64 `json:"fuel_kg"`
	CO2Rate   float64 `json:"g_co2_per_km"`
}

// emissionsTally accumulates the figures one run or one vehicle contributed.
// Distance arrives in metres because that is how SUMO reports it.
type emissionsTally struct {
	trips    int
	distance float64
	co2      float64
	fuel     float64
}

func (t *emissionsTally) add(trip sumo.Trip) {
	t.trips++
	t.distance += trip.DistanceM
	t.co2 += trip.CO2Kg
	t.fuel += trip.FuelKg
}

// summariseEmissions folds the supplied runs into one summary. It treats a run
// as contributing only when it completed and carried a result; queued, running
// and failed runs, and a completion whose result was never stored, say nothing
// about emissions and are skipped. runs is expected newest first, which is the
// order Service.List returns.
func summariseEmissions(runs []simulation.Run) EmissionsSummary {
	summary := EmissionsSummary{
		Runs:     []EmissionsRun{},
		Vehicles: []EmissionsVehicle{},
	}

	var total emissionsTally
	byVehicle := make(map[string]*emissionsTally)

	for _, run := range runs {
		if run.Status != simulation.StatusCompleted || run.Result == nil {
			continue
		}

		var runTally emissionsTally
		for _, trip := range run.Result.Trips {
			if trip.Status != sumo.TripCompleted {
				continue
			}
			runTally.add(trip)
			if trip.VehicleID == "" {
				continue
			}
			vehicle, ok := byVehicle[trip.VehicleID]
			if !ok {
				vehicle = &emissionsTally{}
				byVehicle[trip.VehicleID] = vehicle
			}
			vehicle.add(trip)
		}

		total.trips += runTally.trips
		total.distance += runTally.distance
		total.co2 += runTally.co2
		total.fuel += runTally.fuel
		summary.Runs = append(summary.Runs, EmissionsRun{
			RunID:     run.ID,
			CreatedAt: run.CreatedAt.UTC().Format(time.RFC3339),
			Vehicles:  run.Scenario.Vehicles,
			Trips:     runTally.trips,
			Distance:  round3(runTally.distance / 1000),
			CO2:       round3(runTally.co2),
			Fuel:      round3(runTally.fuel),
			CO2Rate:   round3(co2PerKm(runTally.co2, runTally.distance)),
		})
		// The newest contributing run comes first, so the first one seen sets
		// the method the console names.
		if summary.Method == nil {
			provenance := run.Result.Emissions
			summary.Method = &provenance
		}
	}

	summary.Totals = EmissionsTotals{
		Trips:    total.trips,
		Distance: round3(total.distance / 1000),
		CO2:      round3(total.co2),
		Fuel:     round3(total.fuel),
		CO2Rate:  round3(co2PerKm(total.co2, total.distance)),
		Runs:     len(summary.Runs),
	}

	for id, vehicle := range byVehicle {
		summary.Vehicles = append(summary.Vehicles, EmissionsVehicle{
			VehicleID: id,
			Trips:     vehicle.trips,
			Distance:  round3(vehicle.distance / 1000),
			CO2:       round3(vehicle.co2),
			Fuel:      round3(vehicle.fuel),
			CO2Rate:   round3(co2PerKm(vehicle.co2, vehicle.distance)),
		})
	}
	// Worst emitter first, so the table reads as a ranking. Ties fall back to
	// the vehicle id to keep the order stable between requests.
	sort.Slice(summary.Vehicles, func(i, j int) bool {
		a, b := summary.Vehicles[i], summary.Vehicles[j]
		if a.CO2 != b.CO2 {
			return a.CO2 > b.CO2
		}
		return a.VehicleID < b.VehicleID
	})

	return summary
}

// co2PerKm converts kilograms over metres into grams per kilometre. A run that
// reported no distance has no rate; it is reported as zero rather than a
// division by zero, and no rate is ever NaN or infinite because JSON cannot
// carry either.
func co2PerKm(co2Kg, distanceM float64) float64 {
	if distanceM <= 0 {
		return 0
	}
	rate := co2Kg * 1000 / (distanceM / 1000)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0
	}
	return rate
}

// round3 keeps three decimals, which is finer than any emission figure the
// console shows and short enough that the JSON stays readable.
func round3(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*1000) / 1000
}

func (s *Server) handleEmissionsSummary(w http.ResponseWriter, r *http.Request) {
	if s.simulation == nil {
		writeError(w, r, errNoSimulation)
		return
	}

	runs, err := s.simulation.List(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	// List returns runs without their results, so each completed run is read
	// again to get its trips. A run that cannot be read is skipped: one
	// unreadable run should not cost the client every other run's figures.
	full := make([]simulation.Run, 0, len(runs))
	for _, run := range runs {
		if run.Status != simulation.StatusCompleted {
			continue
		}
		fetched, err := s.simulation.Get(r.Context(), run.ID)
		if err != nil {
			continue
		}
		full = append(full, fetched)
	}

	writeJSON(w, http.StatusOK, summariseEmissions(full))
}
