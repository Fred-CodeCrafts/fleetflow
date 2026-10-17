// Package memory is an in-memory driver.Repository. It backs the service unit
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
	"fleetflow/internal/driver"
)

// Repository keeps drivers in memory, in creation order. It is safe for
// concurrent use, since HTTP handlers and the run loop may read and write at the
// same time.
//
// The store never blocks, so its methods only consult ctx to honour a cancelled
// request; the parameter is otherwise there to satisfy driver.Repository.
type Repository struct {
	mu sync.RWMutex

	// drivers is the insertion order List returns.
	drivers []driver.Driver
	// index maps an id to its position in drivers, so a duplicate create is
	// caught without scanning.
	index map[string]int
}

// New returns an empty Repository.
func New() *Repository {
	return &Repository{index: make(map[string]int)}
}

// Create stores d. A driver whose id is already present fails with a domain
// conflict, mirroring the unique constraint the Postgres schema will carry. A
// driver with no id is a programming error, because the service always issues
// one.
func (r *Repository) Create(ctx context.Context, d driver.Driver) (driver.Driver, error) {
	if err := ctx.Err(); err != nil {
		return driver.Driver{}, err
	}
	if d.ID == "" {
		return driver.Driver{}, domain.Internal("driver_id_required", "driver to store has no id")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.index[d.ID]; exists {
		return driver.Driver{}, domain.Conflict("driver_exists", "driver %s already exists", d.ID)
	}
	r.index[d.ID] = len(r.drivers)
	r.drivers = append(r.drivers, d)
	return d, nil
}

// List returns every stored driver, oldest first. The result is a fresh slice,
// so a caller cannot reach the store through it.
func (r *Repository) List(ctx context.Context) ([]driver.Driver, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	found := make([]driver.Driver, len(r.drivers))
	copy(found, r.drivers)
	return found, nil
}
