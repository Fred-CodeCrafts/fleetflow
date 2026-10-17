package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"fleetflow/internal/domain"
)

// Error codes. "unauthenticated" appears in a 401 body and is part of the
// public contract; the rest are internal and their details never reach a client.
const (
	codeUnauthenticated = "unauthenticated"
	codeNameRequired    = "api_key_name_required"
	codeCreateFailed    = "api_key_create_failed"
	codeLookupFailed    = "api_key_lookup_failed"
	codeIDFailed        = "api_key_id_failed"
	codeCanceled        = "request_cancelled"
)

// ErrUnauthenticated is the single failure Authenticate returns for a presented
// key that is missing, malformed, or simply not stored. It deliberately does not
// distinguish those cases: a caller must not learn whether a key exists, and the
// API maps this one error to 401 and does nothing else (PRD US-13).
//
// It is a domain KindNotFound, so a transport that only knows the domain kinds
// treats it as "no such key", while the API layer can recognise it by identity
// (errors.Is) to return 401 rather than 404.
var ErrUnauthenticated = domain.NotFound(codeUnauthenticated, "missing or invalid API key")

// Key is a stored API key record. It never holds the plaintext key: Hash is the
// SHA-256 digest of the key the caller supplied at issue time, and Prefix is the
// short, non-secret head kept only so a human can tell keys apart. The plaintext
// exists once, in the value Issue returns, and is never written to a record or a
// log.
type Key struct {
	// ID identifies the record, so a key can be named in a log or listed later
	// without quoting its secret.
	ID string
	// Name labels the key for a human, for example "dispatch integration".
	Name string
	// Prefix is the readable, non-secret head of the key.
	Prefix string
	// Hash is SHA-256 of the full plaintext key.
	Hash []byte
	// CreatedAt is when the key was issued.
	CreatedAt time.Time
}

// Repository stores API key records. It is declared here, in the service
// package, so the service owns the seam (DESIGN 3.7): internal/auth/memory backs
// the unit tests, and a Postgres implementation can replace it without the
// service changing.
//
// It deliberately knows nothing about plaintext keys: there is no method that
// accepts or returns one, because none is ever stored.
type Repository interface {
	// Create stores k and returns the stored record. A record whose ID or Hash
	// is already present is a domain conflict, mirroring the unique constraints
	// a Postgres schema will carry.
	Create(ctx context.Context, k Key) (Key, error)

	// ByHash returns the record whose Hash equals hash. No record means a
	// domain error of KindNotFound. That is a storage miss, not an
	// authentication decision: the service makes that call.
	ByHash(ctx context.Context, hash []byte) (Key, error)
}

// IssueInput is one issue request as the transport receives it.
type IssueInput struct {
	// Name labels the key for a human and is required, because a key that cannot
	// be told apart from another is not auditable.
	Name string
}

// Service issues and authenticates API keys on top of a Repository.
type Service struct {
	repo Repository
}

// NewService returns a Service backed by repo.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Issue generates a key, stores its hash, and returns the plaintext key once
// alongside the stored record. The caller hands the plaintext to the user and
// persists only the record.
func (s *Service) Issue(ctx context.Context, in IssueInput) (string, Key, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return "", Key{}, domain.Invalid(codeNameRequired, "api key name is required")
	}

	plaintext, err := Generate()
	if err != nil {
		return "", Key{}, err
	}
	id, err := newID()
	if err != nil {
		return "", Key{}, err
	}

	record := Key{
		ID:        id,
		Name:      name,
		Prefix:    DisplayPrefix(plaintext),
		Hash:      Hash(plaintext),
		CreatedAt: time.Now().UTC(),
	}

	stored, err := s.repo.Create(ctx, record)
	if err != nil {
		return "", Key{}, storageError(codeCreateFailed, err)
	}
	return plaintext, stored, nil
}

// Authenticate checks a presented key against the stored records and returns the
// matching record. Every rejection is the same ErrUnauthenticated, so a missing
// key, a truncated key and an unknown key are indistinguishable to the caller
// and a single 401 mapping covers all of them (PRD US-13).
//
// An empty or malformed key takes the same path as any other: it is hashed and
// looked up, so the work done does not reveal that it was rejected early.
func (s *Service) Authenticate(ctx context.Context, presented string) (Key, error) {
	stored, err := s.repo.ByHash(ctx, Hash(presented))
	if err != nil {
		if domain.KindOf(err) == domain.KindNotFound {
			return Key{}, ErrUnauthenticated
		}
		return Key{}, storageError(codeLookupFailed, err)
	}

	// The store matched by hash; re-check in constant time so a store that
	// matched loosely, or a corrupt row, can never authenticate a key it only
	// partly matches.
	if !Verify(presented, stored.Hash) {
		return Key{}, ErrUnauthenticated
	}
	return stored, nil
}

// idBytes is the size of a generated key id: 128 random bits, written as hex.
const idBytes = 16

// newID returns a random key id. The service issues ids rather than the store,
// so a record is never observable without one and a log can name a key without
// quoting it.
func newID() (string, error) {
	buf := make([]byte, idBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", domain.Internal(codeIDFailed, "could not generate an api key id").WithCause(err)
	}
	return hex.EncodeToString(buf), nil
}

// storageError classifies a repository failure. A domain error the repository
// already classified passes through, because the transport knows how to map it.
// A cancelled or timed-out call becomes unavailable rather than internal: it is
// the request's condition, not a bug. Anything else becomes internal, so a raw
// storage error never reaches a client (DESIGN 3.8).
//
// A store outage must not look like a bad key, so this never produces an
// unauthenticated failure: an unreachable database is a 503, not a 401.
func storageError(code string, err error) *domain.Error {
	var classified *domain.Error
	if errors.As(err, &classified) {
		return classified
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return domain.Unavailable(codeCanceled, "the request was cancelled or timed out").WithCause(err)
	}
	return domain.Internal(code, "api key storage failure").WithCause(err)
}
