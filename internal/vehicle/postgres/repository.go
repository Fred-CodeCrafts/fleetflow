// Package postgres is FleetFlow's PostgreSQL vehicle.Repository: the store that
// makes vehicle records survive a restart, where internal/vehicle/memory is only
// for tests and a database-free run (DESIGN 3.7).
//
// It keeps the same seam as the memory store, so the service neither knows nor
// cares which one backs it. It takes a pool rather than opening a connection, so
// the process owns exactly one pool and this package never closes it.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/domain"
	"fleetflow/internal/vehicle"
)

// uniqueViolation is Postgres' SQLSTATE for a unique-constraint violation. The
// vehicles table's primary key on id is the constraint that produces it, so this
// is how a duplicate create is recognised as the domain conflict the service
// promises the transport.
const uniqueViolation = "23505"

// Repository stores vehicles in the vehicles table, reached through a shared
// pool. It is safe for concurrent use because the pool is.
type Repository struct {
	pool *pgxpool.Pool
}

// New returns a Repository that reads and writes through pool. It takes the
// pool as given: opening and closing connections is the caller's job, so this
// store cannot leak a second pool or shut one down from under its owner.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Create stores v and returns it. A vehicle whose id is already present is a
// domain conflict with the same code the memory store uses, since the primary
// key rejects the insert. Any other failure is returned raw so the service can
// classify it (DESIGN 3.8).
//
// The status is written as its wire name through vehicle.Status.String, never a
// literal, so a token the API accepts is exactly the token the check constraint
// accepts.
func (r *Repository) Create(ctx context.Context, v vehicle.Vehicle) (vehicle.Vehicle, error) {
	const q = `insert into vehicles
		(id, name, type, status, registration, make, model, year, color, body_type, fuel_type, capacity, maintenance_history)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	if _, err := r.pool.Exec(ctx, q, v.ID, v.Name, v.Type, v.Status.String(),
		nullableText(v.Registration), nullableText(v.Make), nullableText(v.Model), v.Year,
		nullableText(v.Color), nullableText(v.BodyType), nullableText(v.FuelType), v.Capacity,
		nullableText(v.MaintenanceHistory)); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return vehicle.Vehicle{}, domain.Conflict("vehicle_exists", "vehicle %s already exists", v.ID)
		}
		return vehicle.Vehicle{}, err
	}
	return v, nil
}

// List returns every stored vehicle matching filter, oldest first. The order is
// created_at then id: rows created in the same instant share a timestamp, and
// the id is the deterministic tiebreaker that keeps the order stable.
//
// A nil filter.Status lists everything; otherwise the query keeps only the rows
// in that status. Statuses are parsed back with vehicle.ParseStatus, the same
// package that wrote them, so the two directions cannot drift.
func (r *Repository) List(ctx context.Context, filter vehicle.ListFilter) ([]vehicle.Vehicle, error) {
	const q = `select id, name, type, status, registration, make, model, year, color, body_type, fuel_type, capacity, maintenance_history from vehicles
		where ($1::text is null or status = $1)
		order by created_at, id`

	var status any
	if filter.Status != nil {
		status = filter.Status.String()
	}

	rows, err := r.pool.Query(ctx, q, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := make([]vehicle.Vehicle, 0)
	for rows.Next() {
		var (
			id, name, typ, statusName                                                string
			registration, make, model, color, bodyType, fuelType, maintenanceHistory *string
			year                                                                     *int
			capacity                                                                 *float64
		)
		if err := rows.Scan(&id, &name, &typ, &statusName, &registration, &make, &model, &year,
			&color, &bodyType, &fuelType, &capacity, &maintenanceHistory); err != nil {
			return nil, err
		}
		st, err := vehicle.ParseStatus(statusName)
		if err != nil {
			return nil, err
		}
		found = append(found, vehicle.Vehicle{
			ID: id, Name: name, Type: typ, Status: st,
			Registration: textValue(registration), Make: textValue(make), Model: textValue(model),
			Year: year, Color: textValue(color), BodyType: textValue(bodyType), FuelType: textValue(fuelType),
			Capacity: capacity, MaintenanceHistory: textValue(maintenanceHistory),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return found, nil
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func textValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
