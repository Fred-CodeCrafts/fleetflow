package api

import (
	"net/http"
	"testing"
	"time"

	"fleetflow/internal/simulation"
	"fleetflow/internal/simulation/sumo"
)

// completedRun builds a completed run carrying the supplied trips, timestamped
// so a test can rely on ordering.
func completedRun(id string, created time.Time, vehicles int, trips ...sumo.Trip) simulation.Run {
	return simulation.Run{
		ID:        id,
		Status:    simulation.StatusCompleted,
		Scenario:  sumo.Scenario{Vehicles: vehicles},
		CreatedAt: created,
		Result: &sumo.Result{
			Emissions: sumo.EmissionProvenance{
				Model:       "HBEFA3",
				Class:       "HBEFA3/PC_G_EU4",
				Source:      "SUMO",
				InputUnit:   "kilometre",
				OutputUnit:  "gram",
				Assumptions: "passenger car, Euro 4",
			},
			Trips: trips,
		},
	}
}

func completedTrip(vehicle string, distanceM, co2Kg, fuelKg float64) sumo.Trip {
	return sumo.Trip{
		VehicleID: vehicle,
		Status:    sumo.TripCompleted,
		DistanceM: distanceM,
		CO2Kg:     co2Kg,
		FuelKg:    fuelKg,
	}
}

// wantFloat compares an aggregated figure with its expected value. The
// aggregates are rounded to three decimals, so that is the tolerance.
func wantFloat(t *testing.T, label string, got, want float64) {
	t.Helper()

	if diff := got - want; diff > 0.0005 || diff < -0.0005 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func TestSummariseEmissions(t *testing.T) {
	newest := time.Date(2026, 10, 25, 1, 0, 0, 0, time.UTC)
	older := newest.Add(-time.Hour)

	runs := []simulation.Run{
		completedRun("run_new", newest, 3,
			completedTrip("FL-01", 1000, 0.2, 0.07),
			completedTrip("FL-02", 3000, 0.9, 0.3),
			// An unattributed trip counts towards the totals but has no vehicle
			// row of its own.
			completedTrip("", 500, 0.1, 0.05),
			// A trip still running reported no distance or emissions yet.
			sumo.Trip{VehicleID: "FL-01", Status: sumo.TripRunning, DistanceM: 9999, CO2Kg: 5},
		),
		completedRun("run_old", older, 1,
			completedTrip("FL-02", 1000, 0.3, 0.1),
		),
		// A failed run may still carry a partial result, but its figures are not
		// trustworthy and are skipped.
		{
			ID:        "run_failed",
			Status:    simulation.StatusFailed,
			CreatedAt: older.Add(-time.Hour),
			Result: &sumo.Result{Trips: []sumo.Trip{
				completedTrip("FL-01", 10000, 9, 3),
			}},
		},
		// Running and completed-without-result runs contribute nothing.
		{ID: "run_running", Status: simulation.StatusRunning, CreatedAt: older.Add(-2 * time.Hour)},
		{ID: "run_empty", Status: simulation.StatusCompleted, CreatedAt: older.Add(-3 * time.Hour)},
	}

	summary := summariseEmissions(runs)

	if summary.Totals.Trips != 4 {
		t.Errorf("totals.trips = %d, want 4", summary.Totals.Trips)
	}
	if summary.Totals.Runs != 2 {
		t.Errorf("totals.runs = %d, want 2", summary.Totals.Runs)
	}
	wantFloat(t, "totals.distance_km", summary.Totals.Distance, 5.5)
	wantFloat(t, "totals.co2_kg", summary.Totals.CO2, 1.5)
	wantFloat(t, "totals.fuel_kg", summary.Totals.Fuel, 0.52)
	wantFloat(t, "totals.g_co2_per_km", summary.Totals.CO2Rate, 272.727)

	if len(summary.Runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(summary.Runs))
	}
	if summary.Runs[0].RunID != "run_new" || summary.Runs[1].RunID != "run_old" {
		t.Errorf("run order = %q, %q; want run_new, run_old",
			summary.Runs[0].RunID, summary.Runs[1].RunID)
	}
	if got := summary.Runs[0].CreatedAt; got != newest.Format(time.RFC3339) {
		t.Errorf("runs[0].created_at = %q, want RFC3339 %q", got, newest.Format(time.RFC3339))
	}
	if summary.Runs[0].Vehicles != 3 {
		t.Errorf("runs[0].vehicles = %d, want 3", summary.Runs[0].Vehicles)
	}
	wantFloat(t, "runs[0].distance_km", summary.Runs[0].Distance, 4.5)
	wantFloat(t, "runs[0].co2_kg", summary.Runs[0].CO2, 1.2)
	wantFloat(t, "runs[0].g_co2_per_km", summary.Runs[0].CO2Rate, 266.667)

	if len(summary.Vehicles) != 2 {
		t.Fatalf("vehicles = %d, want 2 (blank vehicle ids excluded): %v",
			len(summary.Vehicles), summary.Vehicles)
	}
	// FL-02 emitted more, so it leads the ranking.
	if summary.Vehicles[0].VehicleID != "FL-02" || summary.Vehicles[1].VehicleID != "FL-01" {
		t.Errorf("vehicle order = %q, %q; want FL-02, FL-01",
			summary.Vehicles[0].VehicleID, summary.Vehicles[1].VehicleID)
	}
	wantFL02 := summary.Vehicles[0]
	if wantFL02.Trips != 2 {
		t.Errorf("FL-02 trips = %d, want 2", wantFL02.Trips)
	}
	wantFloat(t, "FL-02.distance_km", wantFL02.Distance, 4)
	wantFloat(t, "FL-02.co2_kg", wantFL02.CO2, 1.2)
	wantFloat(t, "FL-02.fuel_kg", wantFL02.Fuel, 0.4)
	wantFloat(t, "FL-02.g_co2_per_km", wantFL02.CO2Rate, 300)

	if summary.Method == nil {
		t.Fatal("method is absent, want the newest contributing run's provenance")
	}
	if summary.Method.Model != "HBEFA3" || summary.Method.Class != "HBEFA3/PC_G_EU4" {
		t.Errorf("method = %+v, want HBEFA3/PC_G_EU4 passed through", *summary.Method)
	}
}

func TestSummariseEmissions_ZeroDistanceIsZeroRate(t *testing.T) {
	summary := summariseEmissions([]simulation.Run{
		completedRun("run_idle", time.Now().UTC(), 1,
			completedTrip("FL-01", 0, 0, 0)),
	})

	if summary.Totals.Trips != 1 {
		t.Fatalf("totals.trips = %d, want 1", summary.Totals.Trips)
	}
	if summary.Totals.CO2Rate != 0 {
		t.Errorf("totals.g_co2_per_km = %v, want 0", summary.Totals.CO2Rate)
	}
	if summary.Vehicles[0].CO2Rate != 0 {
		t.Errorf("vehicle g_co2_per_km = %v, want 0", summary.Vehicles[0].CO2Rate)
	}
	for _, run := range summary.Runs {
		if run.CO2Rate != 0 {
			t.Errorf("run g_co2_per_km = %v, want 0", run.CO2Rate)
		}
	}
}

func TestSummariseEmissions_NoContributingRuns(t *testing.T) {
	summary := summariseEmissions(nil)

	if summary.Totals != (EmissionsTotals{}) {
		t.Errorf("totals = %+v, want zeroes", summary.Totals)
	}
	if len(summary.Runs) != 0 || len(summary.Vehicles) != 0 {
		t.Errorf("runs = %d, vehicles = %d, want both empty",
			len(summary.Runs), len(summary.Vehicles))
	}
	if summary.Method != nil {
		t.Errorf("method = %+v, want absent", *summary.Method)
	}
}

func TestEmissionsSummaryUnavailableWithoutSimulation(t *testing.T) {
	server := testServer(t)

	rec := getJSON(t, server, "/api/emissions/summary")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "simulation_unavailable" {
		t.Errorf("code = %q, want simulation_unavailable", code)
	}
}
