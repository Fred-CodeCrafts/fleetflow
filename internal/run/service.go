package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"fleetflow/internal/domain"
)

// Codes for failures. They travel in API error bodies, so they are part of the
// public contract and must stay stable (see domain.Error and DESIGN 3.8).
const (
	codeConfigRequired  = "run_config_required"
	codeMetricsRequired = "run_metrics_required"
	codeIDRequired      = "run_id_required"
	codeCreateFailed    = "run_create_failed"
	codeListFailed      = "run_list_failed"
	codeGetFailed       = "run_get_failed"
	codeIDFailed        = "run_id_failed"
	codeCanceled        = "request_cancelled"
)

// List pagination. The default is what a dashboard wants on its first paint; the
// cap is what keeps one request from reading the whole table.
const (
	defaultLimit = 25
	maxLimit     = 100
)

// Repository stores and retrieves saved runs. It is declared here, in the service
// package, so the service owns the seam (DESIGN 3.7): internal/run/memory backs
// the unit tests and a database-free run, and a Postgres implementation replaces
// it without the service changing.
//
// Implementations must be safe for concurrent use.
type Repository interface {
	// Create stores r and returns the stored record, so the service never has to
	// guess the instant the store wrote.
	//
	// A run whose id is already present is a domain conflict, mirroring the
	// unique constraint the schema carries.
	Create(ctx context.Context, r Run) (Run, error)

	// List returns up to limit runs, most recent first. The limit is clamped by
	// the service before it is passed on, so an implementation may trust it.
	List(ctx context.Context, limit int) ([]Run, error)

	// Get returns the run with id. No such row is a domain KindNotFound.
	Get(ctx context.Context, id string) (Run, error)
}

// Service applies the run rules on top of a Repository.
type Service struct {
	repo Repository
}

// NewService returns a Service backed by repo.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Save stores one finished run and returns it. The id is minted here and the
// store sets created_at, so the caller learns both from the result.
//
// The seed is passed through unjudged: the simulator defines seeding, so any
// int64 is valid, zero and negative values included, and refusing one would be
// this layer inventing a rule the domain does not have. The two documents are
// required, though: a run saved without its config or its metrics records
// nothing worth keeping, so a blank one is refused before the store is reached.
//
// demo is passed through for the same reason the seed is: whether the deployment
// served the control routes is a fact about where the run happened, and only the
// process knows it.
func (s *Service) Save(ctx context.Context, seed int64, demo bool, config, metrics []byte) (Run, error) {
	if len(config) == 0 {
		return Run{}, domain.Invalid(codeConfigRequired, "run config is required")
	}
	if len(metrics) == 0 {
		return Run{}, domain.Invalid(codeMetricsRequired, "run metrics are required")
	}

	id, err := newID()
	if err != nil {
		return Run{}, err
	}

	stored, err := s.repo.Create(ctx, Run{ID: id, Seed: seed, Demo: demo, Config: config, Metrics: metrics})
	if err != nil {
		return Run{}, storageError(codeCreateFailed, err)
	}
	return stored, nil
}

// List returns up to limit runs, most recent first. The limit is clamped here
// rather than by the store or the handler, so every caller gets a sane page from
// one place: a missing or nonsensical limit becomes the default, and an oversized
// one is capped instead of being refused, because asking for too much is a
// reasonable request that should just be trimmed.
func (s *Service) List(ctx context.Context, limit int) ([]Run, error) {
	found, err := s.repo.List(ctx, clampLimit(limit))
	if err != nil {
		return nil, storageError(codeListFailed, err)
	}
	return found, nil
}

// Get returns the run with id. A blank id is a caller mistake and is refused
// here; a well-formed id that matches no row is the store's not-found, which
// passes through so the transport can map it to 404.
func (s *Service) Get(ctx context.Context, id string) (Run, error) {
	if id == "" {
		return Run{}, domain.Invalid(codeIDRequired, "run id is required")
	}

	found, err := s.repo.Get(ctx, id)
	if err != nil {
		return Run{}, storageError(codeGetFailed, err)
	}
	return found, nil
}

// clampLimit turns a requested page size into one the store can honour: a
// non-positive limit means "the default page", and anything above the cap is
// trimmed to it.
func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// idBytes is the size of a generated run id: 128 random bits, written as hex. The
// service issues ids rather than the store, so a record is never observable
// without one and a row can hold exactly the id the client was already told.
const idBytes = 16

// newID returns a random run id. The "run_" prefix keeps it recognisable in a log
// or a URL, where the kind of thing an id names is not otherwise obvious.
func newID() (string, error) {
	buf := make([]byte, idBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", domain.Internal(codeIDFailed, "could not generate a run id").WithCause(err)
	}
	return "run_" + hex.EncodeToString(buf), nil
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
	return domain.Internal(code, "run storage failure").WithCause(err)
}
