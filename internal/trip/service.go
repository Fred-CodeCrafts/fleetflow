package trip

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"fleetflow/internal/domain"
)

// Codes for failures a client cannot act on: storage or id generation. The
// transport substitutes its own message for these, so the code is what a
// developer debugging a report has to go on.
const (
	codeCreateFailed = "trip_create_failed"
	codeListFailed   = "trip_list_failed"
	codeAssignFailed = "trip_assign_failed"
	codeCanceled     = "request_cancelled"
	codeIDFailed     = "trip_id_failed"
	codeTripRequired = "trip_id_required"
)

// Repository stores and retrieves trips. It is declared here, in the service
// package, so the service owns the seam (DESIGN 3.7): internal/trip/memory
// backs the unit tests and a database-free run, and a Postgres implementation
// can replace it without the service changing.
//
// Implementations must be safe for concurrent use and must return the stored
// record, so the service never has to guess what a store normalised.
type Repository interface {
	// Create stores t and returns the stored record. A trip whose id is
	// already present is a domain conflict, mirroring the unique constraint a
	// Postgres schema will carry.
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
	Assign(ctx context.Context, tripID, vehicleID string) (Trip, error)
}

// CreateInput is one create request as the transport receives it. The status
// travels as its wire name so that parsing and validation live in exactly one
// place, this package, instead of being repeated in every handler.
type CreateInput struct {
	Origin      string
	Destination string
	VehicleID   string
	Status      string
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
