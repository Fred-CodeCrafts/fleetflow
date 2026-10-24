// Package memory is an in-memory run.Repository. It backs the service unit tests
// and lets FleetFlow run without a database.
//
// It is deliberately not a cache and it does not survive a restart, so the
// deployed server uses a Postgres implementation (DESIGN 3.7). Keeping the same
// interface is what lets the service be tested without a database.
package memory

import (
	"context"
	"sync"
	"time"

	"fleetflow/internal/domain"
	"fleetflow/internal/run"
)

// Repository keeps runs in memory, in save order. It is safe for concurrent use,
// since an HTTP handler may save a run while another lists them.
//
// The store never blocks, so its methods only consult ctx to honour a cancelled
// request; the parameter is otherwise there to satisfy run.Repository.
type Repository struct {
	mu sync.RWMutex

	// runs is the save order. List walks it backwards, which is "most recent
	// first" with certainty: the store knows the exact order the runs arrived
	// in, where the database can only order by a timestamp two saves may share.
	runs []run.Run
	// index maps an id to its position in runs, so a duplicate create is caught
	// without scanning.
	index map[string]int
}

// New returns an empty Repository.
func New() *Repository {
	return &Repository{index: make(map[string]int)}
}

// Create stores r and returns it with created_at set. A run whose id is already
// present fails with a domain conflict, mirroring the unique constraint the
// Postgres schema carries. A run with no id is a programming error, because the
// service always issues one, and a blank document is refused for the same reason.
//
// The instant is stamped here, the way the database's own default stamps it, so
// the service learns when the run was saved from the result rather than guessing.
func (r *Repository) Create(ctx context.Context, rec run.Run) (run.Run, error) {
	if err := ctx.Err(); err != nil {
		return run.Run{}, err
	}
	if rec.ID == "" {
		return run.Run{}, domain.Internal("run_id_required", "run to store has no id")
	}
	if len(rec.Config) == 0 {
		return run.Run{}, domain.Internal("run_config_required", "run to store has no config")
	}
	if len(rec.Metrics) == 0 {
		return run.Run{}, domain.Internal("run_metrics_required", "run to store has no metrics")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.index[rec.ID]; exists {
		return run.Run{}, domain.Conflict("run_exists", "run %s already exists", rec.ID)
	}
	rec.CreatedAt = time.Now().UTC()
	stored := clone(rec)
	r.index[stored.ID] = len(r.runs)
	r.runs = append(r.runs, stored)
	return clone(stored), nil
}

// List returns up to limit runs, most recent first. An implementation may trust
// the limit, because the service clamps it before calling.
//
// The result is a fresh slice of fresh records, so a caller cannot reach the
// store through it and cannot mutate a stored document by holding on to the
// bytes it was returned.
func (r *Repository) List(ctx context.Context, limit int) ([]run.Run, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	found := make([]run.Run, 0, len(r.runs))
	for i := len(r.runs) - 1; i >= 0 && len(found) < limit; i-- {
		found = append(found, clone(r.runs[i]))
	}
	return found, nil
}

// Get returns the run with id. Nothing stored under id is a domain KindNotFound,
// the way a missing row reads in SQL.
func (r *Repository) Get(ctx context.Context, id string) (run.Run, error) {
	if err := ctx.Err(); err != nil {
		return run.Run{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	pos, exists := r.index[id]
	if !exists {
		return run.Run{}, domain.NotFound("run_not_found", "run %s does not exist", id)
	}
	return clone(r.runs[pos]), nil
}

// clone copies a record and the two documents it points at, so the store and its
// callers can never share a JSON slice, where a write on one side would be
// visible on the other.
func clone(rec run.Run) run.Run {
	rec.Config = append([]byte(nil), rec.Config...)
	rec.Metrics = append([]byte(nil), rec.Metrics...)
	return rec
}
