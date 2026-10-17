// Package postgres is FleetFlow's PostgreSQL trip.Repository: the store that
// makes trip records survive a restart, where internal/trip/memory is only for
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
	"fleetflow/internal/trip"
	"fleetflow/internal/vehicle"
)

// SQLSTATEs and constraint names this store reacts to. They are the database's
// own vocabulary, so the classification below turns them into the domain errors
// the service promises the transport instead of leaking Postgres' text.
const (
	uniqueViolation     = "23505" // a unique key or index rejected the row
	foreignKeyViolation = "23503" // vehicle_id names a vehicle that does not exist
	checkViolation      = "23514" // a column value is outside its check constraint

	// constraintTripPK is the primary key on trips.id; it is what a duplicate
	// create collides with.
	constraintTripPK = "trips_pkey"

	// Frame the domain errors the same way the memory store does, because the
	// codes travel in API error bodies and are part of the public contract. The
	// memory store cannot see a missing vehicle, so that one code is added here.
	codeTripExists         = "trip_exists"
	codeTripNotFound       = "trip_not_found"
	codeTripCompleted      = "trip_completed"
	codeTripVehicleBusy    = "trip_vehicle_busy"
	codeTripVehicleUnknown = "trip_vehicle_unknown"
	// codeTripVehicleUnavailable is the other half of availability: the vehicle
	// exists but may not take a trip, because it is offline or already on one.
	codeTripVehicleUnavailable = "trip_vehicle_unavailable"
	codeTripIDRequired         = "trip_id_required"
	codeStatusCorrupt          = "trip_status_corrupt"
)

// Repository stores trips in the trips table, reached through a shared pool. It
// is safe for concurrent use because the pool is.
type Repository struct {
	pool *pgxpool.Pool
}

// New returns a Repository that reads and writes through pool. It takes the pool
// as given: opening and closing connections is the caller's job, so this store
// cannot leak a second pool or shut one down from under its owner.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Create stores t and returns it. A trip whose id is already present is a domain
// conflict with the same code the memory store uses, since the primary key
// rejects the insert. Any other failure is returned raw or classified by
// classify, so the service can surface the right error (DESIGN 3.8).
//
// The status is written as its wire name through trip.Status.String, never a
// literal, so a token the API accepts is exactly the token the check constraint
// accepts. An empty vehicle id is stored as NULL, which is how the column says
// "no vehicle"; the empty string is never written.
//
// A trip with no id is a programming error, because the service always issues
// one; the memory store refuses it the same way. A trip that names a vehicle is
// an assignment, so the vehicle is checked in the same transaction as the insert:
// it must exist and be available, and it may be on only one unfinished trip. A
// check followed by a separate write would let two requests both find the same
// vehicle available.
func (r *Repository) Create(ctx context.Context, t trip.Trip) (trip.Trip, error) {
	if t.ID == "" {
		return trip.Trip{}, domain.Internal(codeTripIDRequired, "trip to store has no id")
	}

	// Nothing to check when no vehicle is named.
	if t.VehicleID == "" {
		if err := insertTrip(ctx, r.pool, t); err != nil {
			return trip.Trip{}, classify(t.ID, err)
		}
		return t, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return trip.Trip{}, err
	}
	defer tx.Rollback(ctx)

	// Locking the vehicle row is what serializes two creates that name it; the
	// partial unique index below is the backstop that makes one of them fail even
	// if this read raced ahead of the other.
	var status string
	err = tx.QueryRow(ctx, `select status from vehicles where id = $1 for update`, t.VehicleID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, domain.NotFound(codeTripVehicleUnknown, "vehicle %s does not exist", t.VehicleID)
	}
	if err != nil {
		return trip.Trip{}, err
	}
	if status != vehicle.StatusAvailable.String() {
		return trip.Trip{}, domain.Conflict(codeTripVehicleUnavailable,
			"vehicle %s is not available", t.VehicleID)
	}

	if err := insertTrip(ctx, tx, t); err != nil {
		return trip.Trip{}, classify(t.ID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return trip.Trip{}, err
	}
	return t, nil
}

// execer is the part of a pool and of a transaction that writing a row needs, so
// one statement serves both a plain create and one inside a transaction.
type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// insertTrip writes the trip row, mapping an empty vehicle id to NULL. Whether
// that vehicle may take the trip is the caller's decision; this only stores what
// it is given.
func insertTrip(ctx context.Context, db execer, t trip.Trip) error {
	const q = `insert into trips (id, origin, destination, vehicle_id, status)
		values ($1, $2, $3, $4, $5)`

	var vehicleID any
	if t.VehicleID != "" {
		vehicleID = t.VehicleID
	}

	_, err := db.Exec(ctx, q, t.ID, t.Origin, t.Destination, vehicleID, t.Status.String())
	return err
}

// List returns every stored trip, oldest first. The order is created_at then id:
// rows created in the same instant share a timestamp, and the id is the
// deterministic tiebreaker that keeps the order stable.
//
// Statuses are parsed back with trip.ParseStatus, the same package that wrote
// them, so the two directions cannot drift. A NULL vehicle_id becomes the empty
// string the domain type uses for "no vehicle".
func (r *Repository) List(ctx context.Context) ([]trip.Trip, error) {
	const q = `select id, origin, destination, vehicle_id, status
		from trips
		order by created_at, id`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := make([]trip.Trip, 0)
	for rows.Next() {
		var (
			id, origin, destination, statusName string
			vehicleID                           *string
		)
		if err := rows.Scan(&id, &origin, &destination, &vehicleID, &statusName); err != nil {
			return nil, err
		}
		st, err := trip.ParseStatus(statusName)
		if err != nil {
			return nil, err
		}
		t := trip.Trip{ID: id, Origin: origin, Destination: destination, Status: st}
		if vehicleID != nil {
			t.VehicleID = *vehicleID
		}
		found = append(found, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return found, nil
}

// Assign puts tripID on vehicleID, or takes it off its vehicle when vehicleID is
// empty, and returns the stored trip. It mirrors the memory store's rules: an
// unknown trip is not found, a completed trip is a conflict because it is
// history and neither holds nor accepts a vehicle, a vehicle already on another
// unfinished trip is a conflict, and a vehicle that does not exist is not found.
//
// The change runs in one transaction. Reading the trip FOR UPDATE orders two
// assignments of the same trip, while the availability decision is left to the
// database: trips_one_active_per_vehicle is a partial unique index, so a vehicle
// already on another unfinished trip fails the update with a unique violation
// rather than being missed by a read-then-write. A completed trip is excluded
// from that index, so it does not keep its vehicle busy.
func (r *Repository) Assign(ctx context.Context, tripID, vehicleID string) (trip.Trip, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return trip.Trip{}, err
	}
	defer tx.Rollback(ctx)

	var statusName string
	err = tx.QueryRow(ctx, `select status from trips where id = $1 for update`, tripID).Scan(&statusName)
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, domain.NotFound(codeTripNotFound, "trip %s does not exist", tripID)
	}
	if err != nil {
		return trip.Trip{}, err
	}
	if statusName == trip.StatusCompleted.String() {
		return trip.Trip{}, domain.Conflict(codeTripCompleted,
			"trip %s is completed and is no longer reassigned", tripID)
	}

	var vehicle any
	if vehicleID != "" {
		vehicle = vehicleID
	}

	const q = `update trips set vehicle_id = $2 where id = $1
		returning origin, destination, vehicle_id, status`

	var (
		origin, destination, storedStatus string
		storedVehicle                     *string
	)
	if err := tx.QueryRow(ctx, q, tripID, vehicle).Scan(&origin, &destination, &storedVehicle, &storedStatus); err != nil {
		return trip.Trip{}, classify(tripID, err)
	}

	st, err := trip.ParseStatus(storedStatus)
	if err != nil {
		return trip.Trip{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return trip.Trip{}, err
	}

	t := trip.Trip{ID: tripID, Origin: origin, Destination: destination, Status: st}
	if storedVehicle != nil {
		t.VehicleID = *storedVehicle
	}
	return t, nil
}

// classify turns a driver error into the domain error the service promises the
// transport, or returns the error unchanged so the service classifies it. The
// codes match the memory store, since they are part of the public contract; the
// original cause is kept with WithCause, for logs only, so no Postgres text ever
// reaches a client (DESIGN 3.8).
//
// A unique violation is a duplicate create when it names the primary key, and a
// vehicle already on another unfinished trip otherwise, because the only other
// unique index on trips is the partial one. A foreign key violation can only be
// vehicle_id, so it means the vehicle does not exist. A check violation is a
// value this code should never have written, so it is an internal bug.
func classify(tripID string, err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case uniqueViolation:
		if pgErr.ConstraintName == constraintTripPK {
			return domain.Conflict(codeTripExists, "trip %s already exists", tripID).WithCause(err)
		}
		return domain.Conflict(codeTripVehicleBusy,
			"the vehicle is already on another unfinished trip").WithCause(err)
	case foreignKeyViolation:
		return domain.NotFound(codeTripVehicleUnknown,
			"the vehicle for trip %s does not exist", tripID).WithCause(err)
	case checkViolation:
		return domain.Internal(codeStatusCorrupt,
			"trip %s has a value the schema rejects", tripID).WithCause(err)
	default:
		return err
	}
}
