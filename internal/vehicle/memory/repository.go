// Package memory is an in-memory vehicle.Repository. It backs the service unit
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
	"fleetflow/internal/vehicle"
)

// Repository keeps vehicles in memory, in creation order. It is safe for
// concurrent use, since HTTP handlers and the run loop may read and write at the
// same time.
//
// The store never blocks, so its methods only consult ctx to honour a cancelled
// request; the parameter is otherwise there to satisfy vehicle.Repository.
type Repository struct {
	mu sync.RWMutex

	// vehicles is the insertion order List returns.
	vehicles []vehicle.Vehicle
	// index maps an id to its position in vehicles, so a duplicate create is
	// caught without scanning.
	index map[string]int
}

// New returns an empty Repository.
func New() *Repository {
	return &Repository{index: make(map[string]int)}
}

// Create stores v. A vehicle whose id is already present fails with a domain
// conflict, mirroring the unique constraint the Postgres schema will carry. A
// vehicle with no id is a programming error, because the service always issues
// one.
func (r *Repository) Create(ctx context.Context, v vehicle.Vehicle) (vehicle.Vehicle, error) {
	if err := ctx.Err(); err != nil {
		return vehicle.Vehicle{}, err
	}
	if v.ID == "" {
		return vehicle.Vehicle{}, domain.Internal("vehicle_id_required", "vehicle to store has no id")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.index[v.ID]; exists {
		return vehicle.Vehicle{}, domain.Conflict("vehicle_exists", "vehicle %s already exists", v.ID)
	}
	r.index[v.ID] = len(r.vehicles)
	r.vehicles = append(r.vehicles, v)
	return v, nil
}

// List returns every stored vehicle matching filter, oldest first. The result is
// a fresh slice, so a caller cannot reach the store through it.
func (r *Repository) List(ctx context.Context, filter vehicle.ListFilter) ([]vehicle.Vehicle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	found := make([]vehicle.Vehicle, 0, len(r.vehicles))
	for _, v := range r.vehicles {
		if filter.Status != nil && v.Status != *filter.Status {
			continue
		}
		found = append(found, v)
	}
	return found, nil
}
