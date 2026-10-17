// Package memory is an in-memory trip.Repository. It backs the service unit
// tests and lets FleetFlow run without a database.
//
// It is deliberately not a cache and it does not survive a restart, so the
// deployed server uses a Postgres implementation (DESIGN 3.7). Keeping the same
// interface is what lets the service be tested without a database.
package memory

import (
	"context"
	"sync"

	"fleetflow/internal/domain"
	"fleetflow/internal/trip"
)

// Repository keeps trips in memory, in creation order. It is safe for
// concurrent use, since HTTP handlers and the run loop may read and write at the
// same time.
//
// The store never blocks, so its methods only consult ctx to honour a cancelled
// request; the parameter is otherwise there to satisfy trip.Repository.
type Repository struct {
	mu sync.RWMutex

	// trips is the insertion order List returns.
	trips []trip.Trip
	// index maps an id to its position in trips, so a duplicate create is
	// caught without scanning.
	index map[string]int
}

// New returns an empty Repository.
func New() *Repository {
	return &Repository{index: make(map[string]int)}
}

// Create stores t. A trip whose id is already present fails with a domain
// conflict, mirroring the unique constraint the Postgres schema will carry. A
// trip with no id is a programming error, because the service always issues
// one.
func (r *Repository) Create(ctx context.Context, t trip.Trip) (trip.Trip, error) {
	if err := ctx.Err(); err != nil {
		return trip.Trip{}, err
	}
	if t.ID == "" {
		return trip.Trip{}, domain.Internal("trip_id_required", "trip to store has no id")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.index[t.ID]; exists {
		return trip.Trip{}, domain.Conflict("trip_exists", "trip %s already exists", t.ID)
	}
	r.index[t.ID] = len(r.trips)
	r.trips = append(r.trips, t)
	return t, nil
}

// List returns every stored trip, oldest first. The result is a fresh slice, so
// a caller cannot reach the store through it.
func (r *Repository) List(ctx context.Context) ([]trip.Trip, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	found := make([]trip.Trip, 0, len(r.trips))
	found = append(found, r.trips...)
	return found, nil
}

// Assign puts tripID on vehicleID, or takes it off its vehicle when vehicleID is
// empty, and returns the stored trip.
//
// It enforces availability, which is the rule this store can see in full: a
// vehicle may be on only one unfinished trip at a time. A completed trip does not
// hold its vehicle — it is history — so it neither blocks a new assignment nor
// accepts one.
func (r *Repository) Assign(ctx context.Context, tripID, vehicleID string) (trip.Trip, error) {
	if err := ctx.Err(); err != nil {
		return trip.Trip{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	pos, exists := r.index[tripID]
	if !exists {
		return trip.Trip{}, domain.NotFound("trip_not_found", "trip %s does not exist", tripID)
	}
	if r.trips[pos].Status == trip.StatusCompleted {
		return trip.Trip{}, domain.Conflict("trip_completed",
			"trip %s is completed and is no longer reassigned", tripID)
	}

	if vehicleID != "" {
		for i, t := range r.trips {
			if i != pos && t.VehicleID == vehicleID && t.Status != trip.StatusCompleted {
				return trip.Trip{}, domain.Conflict("trip_vehicle_busy",
					"vehicle %s is already on trip %s", vehicleID, t.ID)
			}
		}
	}

	r.trips[pos].VehicleID = vehicleID
	return r.trips[pos], nil
}
