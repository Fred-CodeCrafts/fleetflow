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
//
// The package deliberately imports neither the simulation service nor the API: it
// takes a run id and a result, which is all it needs, so the dependency runs one
// way and the simulation package never learns that fleet stores exist.
package dispatch

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
// a plate, and demo data rather than something the simulation reported.
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
	number := plateNumber(plate)
	return fleetProfiles[(number-9001)%len(fleetProfiles)]
}

// plateNumber reads the number out of a plate, defaulting to the first plate in
// the sequence when it carries no digits.
func plateNumber(plate string) int {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, plate)
	number, err := strconv.Atoi(digits)
	if err != nil || number < 9001 {
		return 9001
	}
	return number
}

// fleetName is what the register calls the vehicle.
//
// The plate is not a name: it is stored as the registration, and the register
// shows the two side by side. The name is the unit number the fleet works in,
// derived from the plate so the same vehicle keeps the same name on every
// dispatch.
func fleetName(plate string) string {
	return fmt.Sprintf("Unit %d", plateNumber(plate)-9000)
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
// It is idempotent by content, not by run id: a vehicle is matched on the plate
// its registration carries, and a trip is skipped when the same vehicle already
// carries a trip between the same two hubs. Re-running a mode, or restarting the
// server, therefore reuses the rows it created the first time instead of growing
// the register. The presets are covered by a test that proves their hub pairs are
// distinct per vehicle, so the reuse is a decision rather than a collision.
//
// Both stores are read once and indexed, because a dispatch of a hundred vehicles
// would otherwise list the fleet a hundred times.
func (d *Dispatcher) Dispatch(ctx context.Context, runID string, result sumo.Result) error {
	if d == nil || d.fleet == nil || d.trips == nil {
		return nil
	}
	existing, err := d.trips.List(ctx)
	if err != nil {
		return fmt.Errorf("list trips before dispatch: %w", err)
	}
	dispatched := make(map[string]bool, len(existing))
	for _, t := range existing {
		if t.VehicleID == "" {
			continue
		}
		dispatched[tripKey(t.VehicleID, t.Origin, t.Destination)] = true
	}

	vehicles, err := d.vehicleIndex(ctx)
	if err != nil {
		return err
	}

	for _, simulated := range result.Trips {
		if simulated.Status != sumo.TripCompleted {
			continue
		}
		if simulated.Origin == "" || simulated.Destination == "" || simulated.VehicleID == "" {
			continue
		}
		vehicleID, err := d.ensureVehicle(ctx, vehicles, simulated.VehicleID)
		if err != nil {
			return fmt.Errorf("register vehicle %q for run %s: %w", simulated.VehicleID, runID, err)
		}
		key := tripKey(vehicleID, simulated.Origin, simulated.Destination)
		if dispatched[key] {
			continue
		}
		created, err := d.trips.Create(ctx, trip.CreateInput{
			Origin:      simulated.Origin,
			Destination: simulated.Destination,
			VehicleID:   vehicleID,
			Status:      trip.StatusPlanned.String(),
		})
		if err != nil {
			return fmt.Errorf("record trip %q for run %s: %w", simulated.ID, runID, err)
		}
		dispatched[key] = true
		// The scenario reports finished trips, so the record is walked through
		// the states it actually passed rather than inserted as completed: the
		// store derives the timestamps from those transitions, and a completed
		// trip with no start time would be a blank in the history table.
		for _, status := range []string{trip.StatusInProgress.String(), trip.StatusCompleted.String()} {
			if _, err := d.trips.SetStatus(ctx, created.ID, status); err != nil {
				return fmt.Errorf("advance trip %q to %s: %w", created.ID, status, err)
			}
		}
	}
	return nil
}

// vehicleIndex maps a plate, folded to lower case, to its stored vehicle id.
//
// The match is made on the registration, because the plate is the identifier a
// scenario prints. Names are indexed too, so a row registered before plates had
// their own column still resolves instead of being duplicated.
func (d *Dispatcher) vehicleIndex(ctx context.Context) (map[string]string, error) {
	vehicles, err := d.fleet.List(ctx, vehicle.ListFilter{})
	if err != nil {
		return nil, fmt.Errorf("list vehicles: %w", err)
	}
	index := make(map[string]string, len(vehicles)*2)
	for _, v := range vehicles {
		for _, key := range []string{v.Registration, v.Name} {
			if key = strings.ToLower(strings.TrimSpace(key)); key != "" {
				index[key] = v.ID
			}
		}
	}
	return index, nil
}

// ensureVehicle returns the id of the vehicle with this plate, registering it
// when the fleet does not know it yet. The plate becomes the registration; the
// name is the unit the fleet works in.
func (d *Dispatcher) ensureVehicle(ctx context.Context, index map[string]string, plate string) (string, error) {
	plate = strings.TrimSpace(plate)
	key := strings.ToLower(plate)
	if id, ok := index[key]; ok {
		return id, nil
	}
	profile := profileFor(plate)
	year, capacity := profile.Year, profile.Capacity
	created, err := d.fleet.Create(ctx, vehicle.CreateInput{
		Name:         fleetName(plate),
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
	index[key] = created.ID
	return created.ID, nil
}

// tripKey is the identity a trip is deduplicated on: one vehicle, one pair of
// places. The places are folded the same way the stores trim them, so a spelling
// difference cannot smuggle in a duplicate.
func tripKey(vehicleID, origin, destination string) string {
	return vehicleID +
		"\x00" + strings.ToLower(strings.TrimSpace(origin)) +
		"\x00" + strings.ToLower(strings.TrimSpace(destination))
}
