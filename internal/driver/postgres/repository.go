// Package postgres is FleetFlow's PostgreSQL driver.Repository: the store that
// makes driver records survive a restart, where internal/driver/memory is only
// for tests and a database-free run (DESIGN 3.7).
//
// It keeps the same seam as the memory store, so the service neither knows nor
// cares which one backs it. The rules that must survive concurrency — a vehicle
// belongs to at most one driver — are database constraints rather than checks
// here, so two racing requests are stopped whatever the service did before
// them. It takes a pool rather than opening a connection, so the process owns
// exactly one pool and this package never closes it.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/domain"
	"fleetflow/internal/driver"
)

// SQLSTATEs the drivers table can raise. They are matched by code so the
// constraint decides the outcome, and the pgx error text never reaches a client.
const (
	uniqueViolation     = "23505" // a value that must be unique already exists
	foreignKeyViolation = "23503" // a referenced row does not exist
	checkViolation      = "23514" // a value fails a check constraint
)

// Repository stores drivers in the drivers table, reached through a shared
// pool. It is safe for concurrent use because the pool is.
type Repository struct {
	pool *pgxpool.Pool
}

// New returns a Repository that reads and writes through pool. It takes the
// pool as given: opening and closing connections is the caller's job, so this
// store cannot leak a second pool or shut one down from under its owner.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Create stores d and returns it. A driver whose id is already present is a
// domain conflict with the code the memory store uses, since the primary key
// rejects the insert. A vehicle_id that names no vehicle is a not-found, because
// the foreign key rejects it. Any other failure is returned raw so the service
// can classify it (DESIGN 3.8).
func (r *Repository) Create(ctx context.Context, d driver.Driver) (driver.Driver, error) {
	const q = `insert into drivers
		(id, name, contact, vehicle_id, contact_phone, license_class, license_expiry, shift_start, shift_end, availability, contract_type)
		values ($1, $2, $3, $4, $5, $6, $7::text::date, $8::text::time, $9::text::time, $10, $11)`

	if _, err := r.pool.Exec(ctx, q, d.ID, d.Name, d.Contact, nullableVehicle(d.VehicleID),
		nullableText(d.ContactPhone), nullableText(d.LicenseClass), nullableText(d.LicenseExpiry),
		nullableText(d.ShiftStart), nullableText(d.ShiftEnd), nullableText(d.Availability),
		nullableText(d.ContractType)); err != nil {
		return driver.Driver{}, classifyCreate(err, d.ID)
	}
	return d, nil
}

// List returns every stored driver, oldest first. The order is created_at then
// id: rows created in the same instant share a timestamp, and the id is the
// deterministic tiebreaker that keeps the order stable.
func (r *Repository) List(ctx context.Context) ([]driver.Driver, error) {
	const q = `select id, name, contact, vehicle_id, contact_phone, license_class, license_expiry, shift_start, shift_end, availability, contract_type
		from drivers order by created_at, id`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := make([]driver.Driver, 0)
	for rows.Next() {
		var (
			id, name, contact                               string
			vehicle                                         *string
			phone, licenseClass, availability, contractType *string
			licenseExpiry                                   *time.Time
			shiftStart, shiftEnd                            *time.Time
		)
		if err := rows.Scan(&id, &name, &contact, &vehicle, &phone, &licenseClass, &licenseExpiry,
			&shiftStart, &shiftEnd, &availability, &contractType); err != nil {
			return nil, err
		}
		found = append(found, driver.Driver{
			ID:            id,
			Name:          name,
			Contact:       contact,
			ContactPhone:  textValue(phone),
			LicenseClass:  textValue(licenseClass),
			LicenseExpiry: dateValue(licenseExpiry),
			ShiftStart:    clockValue(shiftStart),
			ShiftEnd:      clockValue(shiftEnd),
			Availability:  textValue(availability),
			ContractType:  textValue(contractType),
			VehicleID:     vehicleValue(vehicle),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return found, nil
}

// Assign gives driverID the vehicle vehicleID, or clears its vehicle when
// vehicleID is empty, and returns the stored driver.
//
// It is one statement — update ... returning — so the database's unique index on
// vehicle_id decides who wins a race; a read-then-write here would be exactly
// the race the constraint exists to prevent. Assigning a driver the vehicle it
// already holds is not a conflict, because updating a row to its own value does
// not collide with itself. An unknown driver is a not-found (no row updated), a
// vehicle that names no vehicle is a not-found (foreign key), and a vehicle
// another driver holds is a conflict (unique index).
func (r *Repository) Assign(ctx context.Context, driverID, vehicleID string) (driver.Driver, error) {
	const q = `update drivers set vehicle_id = $2 where id = $1
		returning id, name, contact, vehicle_id, contact_phone, license_class, license_expiry, shift_start, shift_end, availability, contract_type`

	var (
		id, name, contact                               string
		vehicle                                         *string
		phone, licenseClass, availability, contractType *string
		licenseExpiry                                   *time.Time
		shiftStart, shiftEnd                            *time.Time
	)
	err := r.pool.QueryRow(ctx, q, driverID, nullableVehicle(vehicleID)).
		Scan(&id, &name, &contact, &vehicle, &phone, &licenseClass, &licenseExpiry,
			&shiftStart, &shiftEnd, &availability, &contractType)
	if err != nil {
		return driver.Driver{}, classifyAssign(err, driverID, vehicleID)
	}
	return driver.Driver{
		ID:            id,
		Name:          name,
		Contact:       contact,
		ContactPhone:  textValue(phone),
		LicenseClass:  textValue(licenseClass),
		LicenseExpiry: dateValue(licenseExpiry),
		ShiftStart:    clockValue(shiftStart),
		ShiftEnd:      clockValue(shiftEnd),
		Availability:  textValue(availability),
		ContractType:  textValue(contractType),
		VehicleID:     vehicleValue(vehicle),
	}, nil
}

// UpdateProfile replaces profile fields without changing the current vehicle.
func (r *Repository) UpdateProfile(ctx context.Context, driverID string, profile driver.Driver) (driver.Driver, error) {
	const q = `update drivers set name = $2, contact = $3, contact_phone = $4,
		license_class = $5, license_expiry = $6::text::date, shift_start = $7::text::time,
		shift_end = $8::text::time, availability = $9, contract_type = $10
		where id = $1
		returning id, name, contact, vehicle_id, contact_phone, license_class, license_expiry,
		shift_start, shift_end, availability, contract_type`

	var (
		id, name, contact                               string
		vehicle                                         *string
		phone, licenseClass, availability, contractType *string
		licenseExpiry                                   *time.Time
		shiftStart, shiftEnd                            *time.Time
	)
	err := r.pool.QueryRow(ctx, q, driverID, profile.Name, profile.Contact,
		nullableText(profile.ContactPhone), nullableText(profile.LicenseClass),
		nullableText(profile.LicenseExpiry), nullableText(profile.ShiftStart),
		nullableText(profile.ShiftEnd), nullableText(profile.Availability),
		nullableText(profile.ContractType)).Scan(&id, &name, &contact, &vehicle, &phone,
		&licenseClass, &licenseExpiry, &shiftStart, &shiftEnd, &availability, &contractType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return driver.Driver{}, domain.NotFound("driver_not_found", "driver %s does not exist", driverID)
		}
		return driver.Driver{}, err
	}
	return driver.Driver{
		ID: id, Name: name, Contact: contact, VehicleID: vehicleValue(vehicle),
		ContactPhone: textValue(phone), LicenseClass: textValue(licenseClass),
		LicenseExpiry: dateValue(licenseExpiry), ShiftStart: clockValue(shiftStart),
		ShiftEnd: clockValue(shiftEnd), Availability: textValue(availability),
		ContractType: textValue(contractType),
	}, nil
}

// classifyCreate turns a drivers insert failure into a domain error where a
// client can act on it, and passes anything else through for the service to
// classify.
func classifyCreate(err error, id string) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case uniqueViolation:
		return domain.Conflict("driver_exists", "driver %s already exists", id).WithCause(err)
	case foreignKeyViolation:
		return domain.NotFound("driver_vehicle_unknown", "vehicle does not exist").WithCause(err)
	case checkViolation:
		return domain.Internal("driver_constraint_violation", "the driver violates a storage constraint").WithCause(err)
	default:
		return err
	}
}

// classifyAssign turns an assignment failure into a domain error where a client
// can act on it, and passes anything else through for the service to classify.
// No row updated (pgx.ErrNoRows) is an unknown driver: the update matched
// nothing, so the foreign key on the vehicle was never reached.
func classifyAssign(err error, driverID, vehicleID string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotFound("driver_not_found", "driver %s does not exist", driverID)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case foreignKeyViolation:
		return domain.NotFound("driver_vehicle_unknown", "vehicle %s does not exist", vehicleID).WithCause(err)
	case uniqueViolation:
		return domain.Conflict("driver_vehicle_taken", "vehicle %s already belongs to another driver", vehicleID).WithCause(err)
	case checkViolation:
		return domain.Internal("driver_constraint_violation", "the assignment violates a storage constraint").WithCause(err)
	default:
		return err
	}
}

// nullableVehicle maps an empty vehicle id to SQL NULL. A driver with no vehicle
// is a NULL column, never the empty string, so the unique index can allow any
// number of unassigned drivers.
func nullableVehicle(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// vehicleValue maps a NULL column back to the empty vehicle id the domain uses.
func vehicleValue(id *string) string {
	if id == nil {
		return ""
	}
	return *id
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

func dateValue(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}

func clockValue(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("15:04")
}
