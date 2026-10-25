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
	"time"

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
	// codeTripStatusRegressed is a trip being moved backwards in its life, which
	// a finished trip must not be.
	codeTripStatusRegressed = "trip_status_regressed"
	codeTripIDRequired      = "trip_id_required"
	codeStatusCorrupt       = "trip_status_corrupt"
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
//
// A vehicle the trip takes is marked on_trip in the same transaction: a vehicle
// carrying a trip is not available for another one.
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
	// The trip holds the vehicle now, so the vehicle says so. A trip created
	// already completed never held it, so it frees nothing.
	if t.Status != trip.StatusCompleted {
		if err := setVehicleStatus(ctx, tx, t.VehicleID, vehicle.StatusOnTrip); err != nil {
			return trip.Trip{}, err
		}
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
	const q = `insert into trips (id, origin, destination, vehicle_id, status, started_at, finished_at)
		values ($1, $2, $3, $4, $5, $6, $7)`

	var vehicleID, startedAt, finishedAt any
	if t.VehicleID != "" {
		vehicleID = t.VehicleID
	}
	// A caller that knows when the trip ran supplies the times; the status
	// transitions then find them set and leave them alone.
	if !t.StartedAt.IsZero() {
		startedAt = t.StartedAt.UTC()
	}
	if !t.FinishedAt.IsZero() {
		finishedAt = t.FinishedAt.UTC()
	}

	_, err := db.Exec(ctx, q, t.ID, t.Origin, t.Destination, vehicleID, t.Status.String(), startedAt, finishedAt)
	return err
}

// setVehicleStatus writes a vehicle's status, which is how a trip's progress
// reaches the vehicle carrying it. It runs inside the caller's transaction, so
// the trip row and the vehicle row are written together and a process that stops
// between them cannot leave the two disagreeing.
//
// The status is written through the domain's own String, never a literal, so a
// status the API accepts is exactly the status the check constraint accepts.
func setVehicleStatus(ctx context.Context, db execer, vehicleID string, status vehicle.Status) error {
	_, err := db.Exec(ctx, `update vehicles set status = $2 where id = $1`, vehicleID, status.String())
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
	const q = `select id, origin, destination, vehicle_id, status, started_at, finished_at
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
			startedAt, finishedAt               *time.Time
		)
		if err := rows.Scan(&id, &origin, &destination, &vehicleID, &statusName, &startedAt, &finishedAt); err != nil {
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
		// A null timestamp stays the zero value, which is how the domain says
		// "not recorded". Scanning it into a time.Time would fail, and defaulting
		// it to now would invent a duration.
		if startedAt != nil {
			t.StartedAt = *startedAt
		}
		if finishedAt != nil {
			t.FinishedAt = *finishedAt
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
//
// The vehicle the trip leaves is freed and the one it takes is marked on_trip, in
// the same transaction, so a vehicle's status cannot outlive the truth about what
// it is carrying.
func (r *Repository) Assign(ctx context.Context, tripID, vehicleID string) (trip.Trip, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return trip.Trip{}, err
	}
	defer tx.Rollback(ctx)

	var (
		statusName      string
		previousVehicle *string
	)
	err = tx.QueryRow(ctx, `select vehicle_id, status from trips where id = $1 for update`, tripID).
		Scan(&previousVehicle, &statusName)
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

	var assigned any
	if vehicleID != "" {
		assigned = vehicleID
	}

	const q = `update trips set vehicle_id = $2 where id = $1
		returning origin, destination, vehicle_id, status`

	var (
		origin, destination, storedStatus string
		storedVehicle                     *string
	)
	if err := tx.QueryRow(ctx, q, tripID, assigned).Scan(&origin, &destination, &storedVehicle, &storedStatus); err != nil {
		return trip.Trip{}, classify(tripID, err)
	}

	st, err := trip.ParseStatus(storedStatus)
	if err != nil {
		return trip.Trip{}, err
	}

	// The vehicle the trip left is free again, and the one it took is now carrying
	// it. Both writes belong to this transaction, so the trip and the vehicle
	// cannot be left disagreeing.
	if previousVehicle != nil && *previousVehicle != vehicleID {
		if err := setVehicleStatus(ctx, tx, *previousVehicle, vehicle.StatusAvailable); err != nil {
			return trip.Trip{}, err
		}
	}
	if vehicleID != "" {
		if err := setVehicleStatus(ctx, tx, vehicleID, vehicle.StatusOnTrip); err != nil {
			return trip.Trip{}, err
		}
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

// SetStatus moves tripID forwards in its life and returns the stored trip. The
// move runs in one transaction with the trip row locked, so two requests
// advancing the same trip are ordered instead of both reading the old status.
//
// "Forwards" comes from the domain through Status.Advances: a completed trip is
// history, so a move back to planned or in progress is a conflict. Setting the
// status a trip already has is allowed and changes nothing, which keeps a retried
// request harmless. An unknown trip is not found.
func (r *Repository) SetStatus(ctx context.Context, tripID string, status trip.Status) (trip.Trip, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return trip.Trip{}, err
	}
	defer tx.Rollback(ctx)

	var (
		origin, destination, current string
		vehicleID                    *string
	)
	err = tx.QueryRow(ctx,
		`select origin, destination, vehicle_id, status from trips where id = $1 for update`,
		tripID).Scan(&origin, &destination, &vehicleID, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, domain.NotFound(codeTripNotFound, "trip %s does not exist", tripID)
	}
	if err != nil {
		return trip.Trip{}, err
	}

	from, err := trip.ParseStatus(current)
	if err != nil {
		return trip.Trip{}, err
	}
	if !from.Advances(status) {
		return trip.Trip{}, domain.Conflict(codeTripStatusRegressed,
			"trip %s is %s and cannot go back to %s", tripID, from, status)
	}

	// The timestamps belong to the transition, not the request. A trip records
	// its start the first time it reaches in_progress and its finish the first
	// time it reaches completed; a repeated request finds them already set and
	// leaves them alone, so history is not rewritten by a double click.
	//
	// `coalesce` is what makes that one statement: on the first start started_at
	// is null so now() is stored, and on a repeat the existing value wins.
	if _, err := tx.Exec(ctx, `
		update trips set
			status      = $2,
			started_at  = case when $2 = 'in_progress' then coalesce(started_at, now()) else started_at end,
			finished_at = case when $2 = 'completed'   then coalesce(finished_at, now()) else finished_at end
		where id = $1`, tripID, status.String()); err != nil {
		return trip.Trip{}, classify(tripID, err)
	}
	// Finishing is what frees the vehicle: from the moment the trip is completed
	// it no longer holds it, which is what makes the vehicle available for the
	// next one. Only a real move into completed frees anything, so re-setting a
	// completed trip cannot release a vehicle that a later trip has since taken.
	if from != trip.StatusCompleted && status == trip.StatusCompleted && vehicleID != nil {
		if err := setVehicleStatus(ctx, tx, *vehicleID, vehicle.StatusAvailable); err != nil {
			return trip.Trip{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return trip.Trip{}, err
	}

	t := trip.Trip{ID: tripID, Origin: origin, Destination: destination, Status: status}
	if vehicleID != nil {
		t.VehicleID = *vehicleID
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
