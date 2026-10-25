package trip

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"fleetflow/internal/domain"
)

// Codes for failures a client cannot act on: storage or id generation. The
// transport substitutes its own message for these, so the code is what a
// developer debugging a report has to go on.
const (
	codeCreateFailed    = "trip_create_failed"
	codeListFailed      = "trip_list_failed"
	codeAssignFailed    = "trip_assign_failed"
	codeSetStatusFailed = "trip_set_status_failed"
	codeCanceled        = "request_cancelled"
	codeIDFailed        = "trip_id_failed"
	codeTripRequired    = "trip_id_required"
)

// Repository stores and retrieves trips. It is declared here, in the service
// package, so the service owns the seam (DESIGN 3.7): internal/trip/memory
// backs the unit tests and a database-free run, and a Postgres implementation
// can replace it without the service changing.
//
// Implementations must be safe for concurrent use and must return the stored
// record, so the service never has to guess what a store normalised.
type Repository interface {
	// Create stores t and returns the stored record.
	//
	// A trip whose id is already present is a domain conflict, mirroring the
	// unique constraint the schema carries. A trip that names a vehicle is an
	// assignment, so it must satisfy the availability rules: the vehicle must
	// exist, it must be available rather than offline or already on a trip, and
	// it may be on only one unfinished trip. Implementations apply those rules in
	// a single step, because a check followed by a write is exactly the race a
	// concurrent double assignment needs.
	//
	// A memory store holds no vehicles, so it can enforce only the rules it can
	// see; the memory implementation says which it leaves to the database.
	Create(ctx context.Context, t Trip) (Trip, error)

	// List returns every stored trip, oldest first.
	List(ctx context.Context) ([]Trip, error)

	// Assign puts the trip on vehicleID, or takes it off its vehicle when
	// vehicleID is empty, and returns the stored record.
	//
	// Availability is the rule here: a vehicle can be on only one unfinished
	// trip. Implementations must apply the assignment as a single step, so two
	// requests racing for the same vehicle cannot both see it free. A trip that
	// does not exist, a trip that is finished, and a vehicle already on another
	// unfinished trip are reported as domain errors.
	//
	// Assigning marks the vehicle on_trip, and taking the trip off a vehicle
	// makes it available again, so a vehicle's status says whether it is carrying
	// something. Implementations that cannot see vehicles leave that half to the
	// database; the memory implementation says so.
	Assign(ctx context.Context, tripID, vehicleID string) (Trip, error)

	// SetStatus moves the trip to status and returns the stored record, keeping
	// the vehicle carrying it in step: a trip that finishes frees its vehicle,
	// and one that is still going holds on to it.
	//
	// A trip moves forwards only — planned, then in progress, then completed —
	// so a move backwards is a domain conflict. Implementations apply the new
	// status to the trip and to the vehicle in one step, so the two cannot be
	// left disagreeing by a process that stops between them.
	SetStatus(ctx context.Context, tripID string, status Status) (Trip, error)
}

// CreateInput is one create request as the transport receives it. The status
// travels as its wire name so that parsing and validation live in exactly one
// place, this package, instead of being repeated in every handler.
type CreateInput struct {
	Origin      string
	Destination string
	VehicleID   string
	Status      string
	// StartedAt and FinishedAt are for a caller replaying work that already
	// happened, such as a dispatcher importing a finished scenario. Left zero,
	// the status transitions stamp them from the wall clock as usual. Without
	// this, a 26 km trip imported from a run reports a duration of five
	// milliseconds, because that is how long the import took.
	StartedAt  time.Time
	FinishedAt time.Time
}

// Service applies the trip rules on top of a Repository.
type Service struct {
	repo Repository
}

// NewService returns a Service backed by repo.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Create validates in, generates an id and stores the new trip.
func (s *Service) Create(ctx context.Context, in CreateInput) (Trip, error) {
	t, err := New(in.Origin, in.Destination, in.VehicleID, in.Status)
	if err != nil {
		return Trip{}, err
	}
	id, err := newID()
	if err != nil {
		return Trip{}, err
	}
	t.ID = id
	t.StartedAt = in.StartedAt
	t.FinishedAt = in.FinishedAt

	stored, err := s.repo.Create(ctx, t)
	if err != nil {
		return Trip{}, storageError(codeCreateFailed, err)
	}
	return stored, nil
}

// List returns every stored trip, oldest first.
func (s *Service) List(ctx context.Context) ([]Trip, error) {
	found, err := s.repo.List(ctx)
	if err != nil {
		return nil, storageError(codeListFailed, err)
	}
	return found, nil
}

// Assign puts the trip on a vehicle, or takes it off its vehicle when vehicleID
// is empty, and returns the stored trip.
//
// The id is checked here because a blank one is a caller mistake. Availability —
// whether the vehicle is free — belongs to the store, which is the only layer
// that can see the other trips and apply the change in one step.
func (s *Service) Assign(ctx context.Context, tripID, vehicleID string) (Trip, error) {
	tripID = strings.TrimSpace(tripID)
	if tripID == "" {
		return Trip{}, domain.Invalid(codeTripRequired, "trip id is required")
	}

	updated, err := s.repo.Assign(ctx, tripID, strings.TrimSpace(vehicleID))
	if err != nil {
		return Trip{}, storageError(codeAssignFailed, err)
	}
	return updated, nil
}

// SetStatus moves a trip along its life — planned, then in progress, then
// completed — and returns the stored trip.
//
// "Forwards" is the whole rule, and it is the domain's: the store is asked to
// apply a status, not to decide whether the move makes sense. A blank id and an
// unknown status are the client's to fix, so they are refused before the store is
// reached.
func (s *Service) SetStatus(ctx context.Context, tripID, status string) (Trip, error) {
	tripID = strings.TrimSpace(tripID)
	if tripID == "" {
		return Trip{}, domain.Invalid(codeTripRequired, "trip id is required")
	}
	parsed, err := ParseStatus(status)
	if err != nil {
		return Trip{}, err
	}

	updated, err := s.repo.SetStatus(ctx, tripID, parsed)
	if err != nil {
		return Trip{}, storageError(codeSetStatusFailed, err)
	}
	return updated, nil
}

// idBytes is the size of a generated trip id: 128 random bits, written as hex.
// The service issues ids rather than the store, so a record is never observable
// without one and a row can hold exactly the id the client was already told.
const idBytes = 16

// newID returns a random trip id.
func newID() (string, error) {
	buf := make([]byte, idBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", domain.Internal(codeIDFailed, "could not generate a trip id").WithCause(err)
	}
	return hex.EncodeToString(buf), nil
}

// storageError classifies a repository failure. A domain error the repository
// already classified passes through, because the transport knows how to map it.
// A cancelled or timed-out call becomes unavailable rather than internal: it is
// the request's condition, not a bug. Anything else becomes internal, so a raw
// storage error never reaches a client (DESIGN 3.8).
func storageError(code string, err error) *domain.Error {
	var classified *domain.Error
	if errors.As(err, &classified) {
		return classified
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return domain.Unavailable(codeCanceled, "the request was cancelled or timed out").WithCause(err)
	}
	return domain.Internal(code, "trip storage failure").WithCause(err)
}
