package vehicle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"fleetflow/internal/domain"
)

// Codes for failures a client cannot act on: storage or id generation. The
// transport substitutes its own message for these, so the code is what a
// developer debugging a report has to go on.
const (
	codeCreateFailed = "vehicle_create_failed"
	codeListFailed   = "vehicle_list_failed"
	codeCanceled     = "request_cancelled"
	codeIDFailed     = "vehicle_id_failed"
)

// Repository stores and retrieves vehicles. It is declared here, in the service
// package, so the service owns the seam (DESIGN 3.7): internal/vehicle/memory
// backs the unit tests and a database-free run, and a Postgres implementation
// can replace it without the service changing.
//
// Implementations must be safe for concurrent use and must return the stored
// record, so the service never has to guess what a store normalised.
type Repository interface {
	// Create stores v and returns the stored record. A vehicle whose id is
	// already present is a domain conflict, mirroring the unique constraint a
	// Postgres schema will carry.
	Create(ctx context.Context, v Vehicle) (Vehicle, error)

	// List returns every stored vehicle matching filter, oldest first.
	List(ctx context.Context, filter ListFilter) ([]Vehicle, error)
}

// ListFilter narrows a List call. The zero value lists every vehicle.
type ListFilter struct {
	// Status, when not nil, keeps only vehicles in that status.
	Status *Status
}

// ByStatus returns a filter that keeps only vehicles in s. It exists because a
// pointer cannot be taken from a constant directly.
func ByStatus(s Status) ListFilter { return ListFilter{Status: &s} }

// CreateInput is one create request as the transport receives it. The status
// travels as its wire name so that parsing and validation live in exactly one
// place, this package, instead of being repeated in every handler.
type CreateInput struct {
	Name   string
	Type   string
	Status string
}

// Service applies the vehicle rules on top of a Repository.
type Service struct {
	repo Repository
}

// NewService returns a Service backed by repo.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Create validates in, generates an id and stores the new vehicle.
func (s *Service) Create(ctx context.Context, in CreateInput) (Vehicle, error) {
	v, err := New(in.Name, in.Type, in.Status)
	if err != nil {
		return Vehicle{}, err
	}
	id, err := newID()
	if err != nil {
		return Vehicle{}, err
	}
	v.ID = id

	stored, err := s.repo.Create(ctx, v)
	if err != nil {
		return Vehicle{}, storageError(codeCreateFailed, err)
	}
	return stored, nil
}

// List returns the vehicles matching filter.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Vehicle, error) {
	if filter.Status != nil && !filter.Status.Valid() {
		return nil, domain.Invalid(codeStatusUnknown,
			"unknown vehicle status filter %d", uint8(*filter.Status))
	}

	found, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, storageError(codeListFailed, err)
	}
	return found, nil
}

// idBytes is the size of a generated vehicle id: 128 random bits, written as
// hex. The service issues ids rather than the store, so a record is never
// observable without one and a row can hold exactly the id the client was
// already told.
const idBytes = 16

// newID returns a random vehicle id.
func newID() (string, error) {
	buf := make([]byte, idBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", domain.Internal(codeIDFailed, "could not generate a vehicle id").WithCause(err)
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
	return domain.Internal(code, "vehicle storage failure").WithCause(err)
}
