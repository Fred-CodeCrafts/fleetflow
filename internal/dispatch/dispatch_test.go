package dispatch_test

import (
	"context"
	"fmt"
	"testing"

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
		ID:       fmt.Sprintf("veh-%d", f.created),
		Name:     in.Name,
		Type:     in.Type,
		Make:     in.Make,
		Model:    in.Model,
		FuelType: in.FuelType,
		Status:   vehicle.StatusAvailable,
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
