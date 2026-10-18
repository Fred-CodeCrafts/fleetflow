// Package postgres is FleetFlow's PostgreSQL run.Repository: the store that makes
// saved comparison runs survive a restart, where internal/run/memory is only for
// tests and a database-free run (DESIGN 3.7).
//
// It keeps the same seam as the memory store, so the service neither knows nor
// cares which one backs it. It takes a pool rather than opening a connection, so
// the process owns exactly one pool and this package never closes it.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/domain"
	"fleetflow/internal/run"
)

// uniqueViolation is Postgres' SQLSTATE for a unique-constraint violation, which
// only the primary key on runs.id can produce here.
const uniqueViolation = "23505"

// constraintRunPK is the primary key on runs.id; it is what a duplicate create
// collides with.
const constraintRunPK = "runs_pkey"

// Codes for the domain errors this store produces. They are part of the public
// contract: the service passes a classified error through, so its code travels in
// an API error body, and the memory store answers with the same ones.
const (
	codeRunExists   = "run_exists"
	codeRunNotFound = "run_not_found"
)

// Repository stores runs in the runs table, reached through a shared pool. It is
// safe for concurrent use because the pool is.
type Repository struct {
	pool *pgxpool.Pool
}

// New returns a Repository that reads and writes through pool. It takes the pool
// as given: opening and closing connections is the caller's job, so this store
// cannot leak a second pool or shut one down from under its owner.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Create stores r and returns it. A run whose id is already present is a domain
// conflict with the same code the memory store uses, since the primary key
// rejects the insert. Any other failure is returned raw so the service can
// classify it (DESIGN 3.8).
//
// created_at is left to the database default and read back through RETURNING, so
// the returned record carries the instant the store actually wrote rather than
// one this code guessed. The config and metrics are written as the JSON bytes the
// API produced, into jsonb columns: the store never parses or reshapes them.
func (r *Repository) Create(ctx context.Context, rec run.Run) (run.Run, error) {
	const q = `insert into runs (id, seed, demo, config, metrics)
		values ($1, $2, $3, $4, $5)
		returning created_at`

	if err := r.pool.QueryRow(ctx, q, rec.ID, rec.Seed, rec.Demo, rec.Config, rec.Metrics).Scan(&rec.CreatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == constraintRunPK {
			return run.Run{}, domain.Conflict(codeRunExists, "run %s already exists", rec.ID).WithCause(err)
		}
		return run.Run{}, err
	}
	return rec, nil
}

// List returns up to limit runs, most recent first. The order is created_at then
// id: runs saved in the same instant share a timestamp, and the id is the
// deterministic tiebreaker that keeps the order stable, exactly as trips order.
//
// The limit is clamped by the service before it reaches here, so this store
// trusts it rather than second-guessing the caller.
func (r *Repository) List(ctx context.Context, limit int) ([]run.Run, error) {
	const q = `select id, seed, demo, config, metrics, created_at
		from runs
		order by created_at desc, id desc
		limit $1`

	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := make([]run.Run, 0)
	for rows.Next() {
		var rec run.Run
		if err := rows.Scan(&rec.ID, &rec.Seed, &rec.Demo, &rec.Config, &rec.Metrics, &rec.CreatedAt); err != nil {
			return nil, err
		}
		found = append(found, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return found, nil
}

// Get returns the run with id. No row is a domain KindNotFound, the same as the
// memory store, so the transport maps a missing run to 404 the same way whichever
// store backs the service.
func (r *Repository) Get(ctx context.Context, id string) (run.Run, error) {
	const q = `select id, seed, demo, config, metrics, created_at from runs where id = $1`

	var rec run.Run
	err := r.pool.QueryRow(ctx, q, id).Scan(&rec.ID, &rec.Seed, &rec.Demo, &rec.Config, &rec.Metrics, &rec.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return run.Run{}, domain.NotFound(codeRunNotFound, "run %s does not exist", id)
	}
	if err != nil {
		return run.Run{}, err
	}
	return rec, nil
}
