package driver

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
	codeCreateFailed = "driver_create_failed"
	codeListFailed   = "driver_list_failed"
	codeCanceled     = "request_cancelled"
	codeIDFailed     = "driver_id_failed"
)

// Repository stores and retrieves drivers. It is declared here, in the service
// package, so the service owns the seam (DESIGN 3.7): internal/driver/memory
// backs the unit tests and a database-free run, and a Postgres implementation
// can replace it without the service changing.
//
// Implementations must be safe for concurrent use and must return the stored
// record, so the service never has to guess what a store normalised.
type Repository interface {
	// Create stores d and returns the stored record. A driver whose id is
	// already present is a domain conflict, mirroring the unique constraint a
	// Postgres schema will carry.
	Create(ctx context.Context, d Driver) (Driver, error)

	// List returns every stored driver, oldest first.
	List(ctx context.Context) ([]Driver, error)
}

// CreateInput is one create request as the transport receives it, so that
// validation lives in exactly one place, this package, instead of being
// repeated in every handler.
type CreateInput struct {
	Name    string
	Contact string
}

// Service applies the driver rules on top of a Repository.
type Service struct {
	repo Repository
}

// NewService returns a Service backed by repo.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Create validates in, generates an id and stores the new driver.
func (s *Service) Create(ctx context.Context, in CreateInput) (Driver, error) {
	d, err := New(in.Name, in.Contact)
	if err != nil {
		return Driver{}, err
	}
	id, err := newID()
	if err != nil {
		return Driver{}, err
	}
	d.ID = id

	stored, err := s.repo.Create(ctx, d)
	if err != nil {
		return Driver{}, storageError(codeCreateFailed, err)
	}
	return stored, nil
}

// List returns every stored driver, oldest first.
func (s *Service) List(ctx context.Context) ([]Driver, error) {
	found, err := s.repo.List(ctx)
	if err != nil {
		return nil, storageError(codeListFailed, err)
	}
	return found, nil
}

// idBytes is the size of a generated driver id: 128 random bits, written as
// hex. The service issues ids rather than the store, so a record is never
// observable without one and a row can hold exactly the id the client was
// already told.
const idBytes = 16

// newID returns a random driver id.
func newID() (string, error) {
	buf := make([]byte, idBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", domain.Internal(codeIDFailed, "could not generate a driver id").WithCause(err)
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
	return domain.Internal(code, "driver storage failure").WithCause(err)
}
