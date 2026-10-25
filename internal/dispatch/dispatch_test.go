package dispatch_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"fleetflow/internal/dispatch"
	"fleetflow/internal/simulation/sumo"
	"fleetflow/internal/trip"
	"fleetflow/internal/vehicle"
)

type fakeFleet struct {
	vehicles []vehicle.Vehicle
	created  int
}

func (f *fakeFleet) List(context.Context, vehicle.ListFilter) ([]vehicle.Vehicle, error) {
	return append([]vehicle.Vehicle(nil), f.vehicles...), nil
}

func (f *fakeFleet) Create(_ context.Context, in vehicle.CreateInput) (vehicle.Vehicle, error) {
	f.created++
	v := vehicle.Vehicle{
		ID:           fmt.Sprintf("veh-%d", f.created),
		Name:         in.Name,
		Type:         in.Type,
		Registration: in.Registration,
		Make:         in.Make,
		Model:        in.Model,
		FuelType:     in.FuelType,
		Status:       vehicle.StatusAvailable,
	}
	f.vehicles = append(f.vehicles, v)
	return v, nil
}

type fakeTrips struct {
	trips    []trip.Trip
	nextID   int
	statuses []string
}

func (f *fakeTrips) List(context.Context) ([]trip.Trip, error) {
	return append([]trip.Trip(nil), f.trips...), nil
}

func (f *fakeTrips) Create(_ context.Context, in trip.CreateInput) (trip.Trip, error) {
	f.nextID++
	t := trip.Trip{
		ID:          fmt.Sprintf("trip-%d", f.nextID),
		Origin:      in.Origin,
		Destination: in.Destination,
		VehicleID:   in.VehicleID,
		StartedAt:   in.StartedAt,
		FinishedAt:  in.FinishedAt,
	}
	f.trips = append(f.trips, t)
	return t, nil
}

func (f *fakeTrips) SetStatus(_ context.Context, tripID, status string) (trip.Trip, error) {
	f.statuses = append(f.statuses, status)
	for i, t := range f.trips {
		if t.ID == tripID {
			return f.trips[i], nil
		}
	}
	return trip.Trip{}, fmt.Errorf("trip %q not found", tripID)
}

func result() sumo.Result {
	return sumo.Result{Trips: []sumo.Trip{
		{ID: "t1", VehicleID: "B-9001-JKT", Origin: "Tanjung Priok Port Terminal", Destination: "Glodok Commercial District", Status: sumo.TripCompleted},
		{ID: "t2", VehicleID: "B-9002-JKT", Origin: "Pluit Urban Logistics Depot", Destination: "Senayan City Retail Center", Status: sumo.TripCompleted},
		// A running trip has not arrived, so it is not history yet.
		{ID: "t3", VehicleID: "B-9003-JKT", Origin: "Halim Air Cargo Terminal", Destination: "Sudirman Business Corridor", Status: sumo.TripRunning},
		// A trip without a named place cannot be recorded as a dispatch.
		{ID: "t4", VehicleID: "B-9004-JKT", Status: sumo.TripCompleted},
	}}
}

func TestDispatchRecordsVehiclesAndTripsOnce(t *testing.T) {
	fleet := &fakeFleet{}
	trips := &fakeTrips{}
	d := dispatch.New(fleet, trips)

	if err := d.Dispatch(context.Background(), "sim_test", result()); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if fleet.created != 2 {
		t.Fatalf("registered %d vehicles, want 2", fleet.created)
	}
	if len(trips.trips) != 2 {
		t.Fatalf("recorded %d trips, want 2", len(trips.trips))
	}
	if len(trips.statuses) != 4 {
		t.Fatalf("walked %d statuses, want in_progress and completed for each of 2 trips", len(trips.statuses))
	}
	if trips.trips[0].VehicleID != "veh-1" {
		t.Fatalf("trip vehicle = %q, want the registered vehicle id", trips.trips[0].VehicleID)
	}
	// A registered vehicle needs a specification the register can show, and it
	// must be derived from the plate so the same vehicle is described the same
	// way on every dispatch.
	if fleet.vehicles[0].Make == "" || fleet.vehicles[0].Model == "" || fleet.vehicles[0].Type == "" {
		t.Fatalf("registered vehicle has no profile: %#v", fleet.vehicles[0])
	}
	// The plate is the registration, not the name: the register shows a unit and
	// its plate as separate fields.
	if fleet.vehicles[0].Registration != "B-9001-JKT" {
		t.Fatalf("registration = %q, want the plate", fleet.vehicles[0].Registration)
	}
	if fleet.vehicles[0].Name == fleet.vehicles[0].Registration {
		t.Fatalf("name = %q, want a unit name rather than the plate", fleet.vehicles[0].Name)
	}
	if fleet.vehicles[0].Name != "Unit 1" {
		t.Fatalf("name = %q, want the unit number from the plate", fleet.vehicles[0].Name)
	}
	// The profile follows the plate, so two plates get two specifications.
	if fleet.vehicles[0].Model == fleet.vehicles[1].Model {
		t.Fatalf("both vehicles got model %q, want a profile each", fleet.vehicles[0].Model)
	}

	// Dispatching the same scenario again must not grow the fleet or duplicate
	// the history: a restart is not a second run.
	if err := d.Dispatch(context.Background(), "sim_test", result()); err != nil {
		t.Fatalf("second Dispatch failed: %v", err)
	}
	if fleet.created != 2 || len(trips.trips) != 2 {
		t.Fatalf("second dispatch created %d vehicles and %d trips, want 2 and 2", fleet.created, len(trips.trips))
	}
}

func TestDispatchWithoutStoresIsANoOp(t *testing.T) {
	var d *dispatch.Dispatcher
	if err := d.Dispatch(context.Background(), "sim", result()); err != nil {
		t.Fatalf("nil dispatcher returned error: %v", err)
	}
	if err := dispatch.New(nil, nil).Dispatch(context.Background(), "sim", result()); err != nil {
		t.Fatalf("empty dispatcher returned error: %v", err)
	}
}

// A dispatched trip carries the scenario's clock, not the import's. Without
// this, a 26 km trip imported a week later reports a duration of a few
// milliseconds, which is how long the import took.
func TestDispatchKeepsTheScenarioClock(t *testing.T) {
	fleet := &fakeFleet{}
	trips := &fakeTrips{}
	dispatcher := dispatch.New(fleet, trips)

	started := time.Date(2026, 10, 25, 6, 0, 0, 0, time.UTC)
	arrival := started.Add(42 * time.Minute).Sub(started).Seconds()
	out := sumo.Result{
		StartedAt: started,
		Trips: []sumo.Trip{{
			ID:          "t1",
			VehicleID:   "B-9001-JKT",
			Origin:      "Tanjung Priok Port Terminal",
			Destination: "Glodok Commercial District",
			Status:      sumo.TripCompleted,
			DepartS:     600,
			ArrivalS:    &arrival,
			DurationS:   2520,
		}},
	}

	if err := dispatcher.Dispatch(context.Background(), "run-1", out); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(trips.trips) != 1 {
		t.Fatalf("recorded %d trips, want 1", len(trips.trips))
	}
	recorded := trips.trips[0]
	if want := started.Add(10 * time.Minute); !recorded.StartedAt.Equal(want) {
		t.Errorf("started at %v, want %v", recorded.StartedAt, want)
	}
	if want := started.Add(42 * time.Minute); !recorded.FinishedAt.Equal(want) {
		t.Errorf("finished at %v, want %v", recorded.FinishedAt, want)
	}
	if duration, ok := recorded.Duration(); !ok || duration != 32*time.Minute {
		t.Errorf("duration = %v (reported %v), want 32m", duration, ok)
	}
}
