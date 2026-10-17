// Package memory is an in-memory auth.Repository. It backs the auth service
// unit tests and lets FleetFlow run without a database.
//
// It is deliberately not a cache and it does not survive a restart, so the
// deployed server uses a Postgres implementation (DESIGN 3.7). It stores exactly
// what auth.Key holds, so it never holds a plaintext key either.
package memory

import (
	"context"
	"sync"

	"fleetflow/internal/auth"
	"fleetflow/internal/domain"
)

// Repository keeps API key records in memory, in creation order. It is safe for
// concurrent use, since HTTP handlers may authenticate while startup code issues
// a new key.
//
// The store never blocks, so its methods only consult ctx to honour a cancelled
// request; the parameter is otherwise there to satisfy auth.Repository.
type Repository struct {
	mu sync.RWMutex

	// keys is the insertion order List returns.
	keys []auth.Key
	// byID and byHash index a record, so a duplicate create or an authenticate
	// is caught without scanning: the two unique constraints a Postgres schema
	// will carry.
	byID   map[string]int
	byHash map[string]int
}

// New returns an empty Repository.
func New() *Repository {
	return &Repository{byID: make(map[string]int), byHash: make(map[string]int)}
}

// Create stores k. A record whose id or hash is already present fails with a
// domain conflict, mirroring the unique constraints the Postgres schema will
// carry. A record with no id or no hash is a programming error, because the
// service always issues both.
func (r *Repository) Create(ctx context.Context, k auth.Key) (auth.Key, error) {
	if err := ctx.Err(); err != nil {
		return auth.Key{}, err
	}
	if k.ID == "" {
		return auth.Key{}, domain.Internal("api_key_id_required", "api key to store has no id")
	}
	if len(k.Hash) == 0 {
		return auth.Key{}, domain.Internal("api_key_hash_required", "api key to store has no hash")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byID[k.ID]; exists {
		return auth.Key{}, domain.Conflict("api_key_exists", "api key %s already exists", k.ID)
	}
	if _, exists := r.byHash[string(k.Hash)]; exists {
		return auth.Key{}, domain.Conflict("api_key_hash_exists", "an api key with this hash already exists")
	}

	stored := clone(k)
	r.keys = append(r.keys, stored)
	pos := len(r.keys) - 1
	r.byID[stored.ID] = pos
	r.byHash[string(stored.Hash)] = pos
	return clone(stored), nil
}

// ByHash returns the record whose hash equals hash. Nothing stored under hash is
// a domain KindNotFound, the way a missing row reads in SQL; the service turns
// that into an authentication failure.
func (r *Repository) ByHash(ctx context.Context, hash []byte) (auth.Key, error) {
	if err := ctx.Err(); err != nil {
		return auth.Key{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	pos, ok := r.byHash[string(hash)]
	if !ok {
		return auth.Key{}, domain.NotFound("api_key_not_found", "no api key with this hash")
	}
	return clone(r.keys[pos]), nil
}

// List returns every stored record, oldest first. It is not part of
// auth.Repository: it exists so tests, and later a list endpoint, can see the
// store without already holding a lookup key. The result is a fresh copy, so a
// caller cannot reach the store through it.
func (r *Repository) List(ctx context.Context) ([]auth.Key, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	found := make([]auth.Key, 0, len(r.keys))
	for _, k := range r.keys {
		found = append(found, clone(k))
	}
	return found, nil
}

// clone copies a record and its hash, so the store and its callers can never
// share the digest slice.
func clone(k auth.Key) auth.Key {
	k.Hash = append([]byte(nil), k.Hash...)
	return k
}
