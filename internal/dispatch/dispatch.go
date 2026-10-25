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
	"strings"
	"time"

	"fleetflow/internal/fleet"
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
		// The scenario knows when the trip ran; the import does not. Stamping the
		// import's own wall clock would report a 26 km trip as the five
		// milliseconds the replay took, which is exactly what the console showed.
		startedAt, finishedAt := scenarioWindow(result.StartedAt, simulated)
		created, err := d.trips.Create(ctx, trip.CreateInput{
			Origin:      simulated.Origin,
			Destination: simulated.Destination,
			VehicleID:   vehicleID,
			Status:      trip.StatusPlanned.String(),
			StartedAt:   startedAt,
			FinishedAt:  finishedAt,
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
	profile := fleet.For(plate)
	year, capacity := profile.Year, profile.CapacityKg
	created, err := d.fleet.Create(ctx, vehicle.CreateInput{
		Name:         fleet.Name(plate),
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

// scenarioWindow returns when a scenario trip started and finished, in the run's
// own clock. An unknown start time stays zero, so the status transitions fall
// back to the wall clock rather than recording an invented time.
func scenarioWindow(started time.Time, simulated sumo.Trip) (time.Time, time.Time) {
	if started.IsZero() {
		return time.Time{}, time.Time{}
	}
	startedAt := started.Add(seconds(simulated.DepartS))
	switch {
	case simulated.ArrivalS != nil:
		return startedAt, started.Add(seconds(*simulated.ArrivalS))
	case simulated.DurationS > 0:
		return startedAt, startedAt.Add(seconds(simulated.DurationS))
	}
	return startedAt, time.Time{}
}

// seconds converts the scenario's wall-clock seconds into a duration.
func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}
