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
	"strconv"
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

// fleetProfile is the specification a dispatched vehicle is registered with.
//
// The scenario decides where and how fast a vehicle drives, not what it is, so
// these are assigned by the vehicle's position in the fleet sequence: stable for
// a plate, and demo data rather than something the simulation reported. They
// mirror the profiles in scripts/dev/seed-fleet.ps1 so a seeded fleet and a
// dispatched fleet describe the same vehicles.
type fleetProfile struct {
	Make     string
	Model    string
	Type     string
	BodyType string
	FuelType string
	Color    string
	Year     int
	Capacity float64
}

// fleetProfiles are the seven Jakarta light-commercial profiles a dispatched
// plate can be registered with.
var fleetProfiles = []fleetProfile{
	{Make: "Isuzu", Model: "Giga FVM Heavy Cargo", Type: "truck", BodyType: "Heavy Cargo Box", FuelType: "diesel", Color: "White", Year: 2022, Capacity: 14000},
	{Make: "Mitsubishi", Model: "Fuso Canter 136 HDL", Type: "truck", BodyType: "Box Truck", FuelType: "diesel", Color: "Yellow", Year: 2023, Capacity: 4500},
	{Make: "Toyota", Model: "HiAce Commuter Cargo", Type: "van", BodyType: "Van", FuelType: "diesel", Color: "Silver", Year: 2023, Capacity: 1200},
	{Make: "Daihatsu", Model: "Gran Max Blind Van", Type: "van", BodyType: "Blind Van", FuelType: "petrol", Color: "White", Year: 2024, Capacity: 800},
	{Make: "Honda", Model: "PCX Electric Cargo", Type: "motorcycle", BodyType: "Motorcycle Cargo", FuelType: "electric", Color: "Blue", Year: 2024, Capacity: 60},
	{Make: "Hino", Model: "Dutro 130 HD Cargo", Type: "truck", BodyType: "Box Truck", FuelType: "diesel", Color: "Green", Year: 2023, Capacity: 4500},
	{Make: "Wuling", Model: "Formo Blind Van", Type: "van", BodyType: "Blind Van", FuelType: "petrol", Color: "White", Year: 2023, Capacity: 650},
}

// profileFor returns the profile for a plate, chosen from its number so the same
// vehicle always receives the same specification.
func profileFor(plate string) fleetProfile {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, plate)
	number, err := strconv.Atoi(digits)
	if err != nil || number < 9001 {
		number = 9001
	}
	return fleetProfiles[(number-9001)%len(fleetProfiles)]
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
	profile := profileFor(plate)
	year, capacity := profile.Year, profile.Capacity
	created, err := d.fleet.Create(ctx, vehicle.CreateInput{
		Name:         plate,
		Type:         profile.Type,
		Status:       vehicle.StatusAvailable.String(),
		Registration: plate,
		Make:         profile.Make,
		Model:        profile.Model,
		Year:         &year,
		Color:        profile.Color,
		BodyType:     profile.BodyType,
		FuelType:     profile.FuelType,
		Capacity:     &capacity,
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
