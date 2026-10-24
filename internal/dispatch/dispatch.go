// Package dispatch turns a finished simulation scenario into fleet records.
//
// Why it exists: a scenario that reports "100 vehicles" is only comparable with
// the rest of the console if those vehicles exist. Without this, the Vehicles
// page lists a demo fleet, the Trips page lists hand-created dispatches, and the
// scenario's 100 vehicles live in a JSON blob nobody can open. After this, a run
// registers the vehicles it used and one trip per simulated trip, with the real
// origin and destination hubs, so Vehicles, Trips and the run detail all describe
// the same dispatch.
//
// What it is not: a scheduler. It records what the scenario already decided; it
// assigns no drivers and plans no new work.
package dispatch

import (
	"context"
	"fmt"
	"strings"

	"fleetflow/internal/simulation"
	"fleetflow/internal/simulation/sumo"
	"fleetflow/internal/trip"
	"fleetflow/internal/vehicle"
)

// Fleet is the slice of the vehicle service this package needs.
type Fleet interface {
	List(ctx context.Context, filter vehicle.ListFilter) ([]vehicle.Vehicle, error)
	Create(ctx context.Context, in vehicle.CreateInput) (vehicle.Vehicle, error)
}

// Trips is the slice of the trip service this package needs.
type Trips interface {
	List(ctx context.Context) ([]trip.Trip, error)
	Create(ctx context.Context, in trip.CreateInput) (trip.Trip, error)
	SetStatus(ctx context.Context, tripID, status string) (trip.Trip, error)
}

// Dispatcher writes a scenario's vehicles and trips into the fleet stores.
type Dispatcher struct {
	fleet Fleet
	trips Trips
}

// New returns a Dispatcher over the given stores.
func New(fleet Fleet, trips Trips) *Dispatcher {
	return &Dispatcher{fleet: fleet, trips: trips}
}

// Dispatch registers every completed trip's vehicle and then the trip itself.
//
// It is idempotent by content, not by run id: a vehicle is matched on its plate,
// and a trip is skipped when the same vehicle already carries a trip between the
// same two hubs. Re-running a mode, or restarting the server, therefore reuses
// the rows it created the first time instead of growing the register.
//
// Vehicles are registered as vans. The scenario decides how far and how fast a
// vehicle drives, not what it is, so claiming a heavier class here would be
// inventing a specification.
func (d *Dispatcher) Dispatch(ctx context.Context, run simulation.Run, result sumo.Result) error {
	if d == nil || d.fleet == nil || d.trips == nil {
		return nil
	}
	existing, err := d.trips.List(ctx)
	if err != nil {
		return fmt.Errorf("list trips before dispatch: %w", err)
	}

	for _, simulated := range result.Trips {
		if simulated.Status != sumo.TripCompleted {
			continue
		}
		if simulated.Origin == "" || simulated.Destination == "" || simulated.VehicleID == "" {
			continue
		}
		vehicleID, err := d.ensureVehicle(ctx, simulated.VehicleID)
		if err != nil {
			return fmt.Errorf("register vehicle %q for run %s: %w", simulated.VehicleID, run.ID, err)
		}
		if alreadyDispatched(existing, vehicleID, simulated.Origin, simulated.Destination) {
			continue
		}
		created, err := d.trips.Create(ctx, trip.CreateInput{
			Origin:      simulated.Origin,
			Destination: simulated.Destination,
			VehicleID:   vehicleID,
			Status:      trip.StatusPlanned.String(),
		})
		if err != nil {
			return fmt.Errorf("record trip %q for run %s: %w", simulated.ID, run.ID, err)
		}
		// The scenario reports finished trips, so the record is walked through
		// the states it actually passed rather than inserted as completed: the
		// store derives the timestamps from those transitions, and a completed
		// trip with no start time would be a blank in the history table.
		for _, status := range []string{trip.StatusInProgress.String(), trip.StatusCompleted.String()} {
			if _, err := d.trips.SetStatus(ctx, created.ID, status); err != nil {
				return fmt.Errorf("advance trip %q to %s: %w", created.ID, status, err)
			}
		}
		existing = append(existing, created)
	}
	return nil
}

// ensureVehicle returns the id of the vehicle with this plate, registering it
// when the fleet does not know it yet.
func (d *Dispatcher) ensureVehicle(ctx context.Context, plate string) (string, error) {
	plate = strings.TrimSpace(plate)
	vehicles, err := d.fleet.List(ctx, vehicle.ListFilter{})
	if err != nil {
		return "", fmt.Errorf("list vehicles: %w", err)
	}
	for _, v := range vehicles {
		if strings.EqualFold(v.Name, plate) {
			return v.ID, nil
		}
	}
	created, err := d.fleet.Create(ctx, vehicle.CreateInput{
		Name:   plate,
		Type:   "van",
		Status: vehicle.StatusAvailable.String(),
	})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// alreadyDispatched reports whether this vehicle already carries a trip between
// these two places.
func alreadyDispatched(trips []trip.Trip, vehicleID, origin, destination string) bool {
	for _, t := range trips {
		if t.VehicleID != vehicleID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(t.Origin), origin) &&
			strings.EqualFold(strings.TrimSpace(t.Destination), destination) {
			return true
		}
	}
	return false
}
