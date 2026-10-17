// Package postgres is FleetFlow's PostgreSQL auth.Repository: the store that
// makes API key records survive a restart, where internal/auth/memory is only
// for tests and a database-free run (DESIGN 3.7).
//
// It keeps the same seam as the memory store, so the service neither knows nor
// cares which one backs it. It takes a pool rather than opening a connection, so
// the process owns exactly one pool and this package never closes it.
//
// A record holds only the digest of a key, never the plaintext, so this store
// has nothing secret to read back and nothing secret to log.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/auth"
	"fleetflow/internal/domain"
)

// uniqueViolation is Postgres' SQLSTATE for a unique-constraint violation. Both
// api_keys' primary key on id and its unique constraint on hash produce it, so
// this is how a duplicate create is recognised as the domain conflict the
// service promises the transport.
const uniqueViolation = "23505"

// constraintHashUnique is the name of the unique constraint on hash. It is how a
// repeated hash is told apart from a repeated id, since both arrive as the same
// SQLSTATE.
const constraintHashUnique = "api_keys_hash_unique"

// Codes for the domain errors this store produces. They are part of the public
// contract: the service passes a classified error through, so its code travels in
// an API error body, and the memory store answers with the same ones.
const (
	// codeAPIKeyExists is a reused id: the caller is storing a record that is
	// already there under the same name.
	codeAPIKeyExists = "api_key_exists"
	// codeAPIKeyHashExists is a repeated hash, which means the same key material
	// was generated twice. With 256 bits of randomness that should not happen at
	// all, so it is reported apart from an id collision the caller would go
	// looking for and never find.
	codeAPIKeyHashExists = "api_key_hash_exists"
	codeAPIKeyNotFound   = "api_key_not_found"
)

// Repository stores API keys in the api_keys table, reached through a shared
// pool. It is safe for concurrent use because the pool is.
type Repository struct {
	pool *pgxpool.Pool
}

// New returns a Repository that reads and writes through pool. It takes the pool
// as given: opening and closing connections is the caller's job, so this store
// cannot leak a second pool or shut one down from under its owner.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Create stores k and returns the stored record. A record whose id or hash is
// already present is a domain conflict, since each is backed by a unique
// constraint in the schema; the memory store refuses them the same way, with the
// same codes. The two are told apart because they are different mistakes: a
// reused id is the caller's, while a repeated hash means the same key material was
// generated twice. Any other failure is returned raw so the service can classify
// it (DESIGN 3.8).
//
// created_at is left to the database default and read back through RETURNING,
// so the returned record carries the instant the store actually wrote rather
// than one this code guessed. The plaintext key is never passed here: k.Hash is
// the digest the service already computed.
func (r *Repository) Create(ctx context.Context, k auth.Key) (auth.Key, error) {
	const q = `insert into api_keys (id, name, prefix, hash)
		values ($1, $2, $3, $4)
		returning created_at`

	if err := r.pool.QueryRow(ctx, q, k.ID, k.Name, k.Prefix, k.Hash).Scan(&k.CreatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			if pgErr.ConstraintName == constraintHashUnique {
				return auth.Key{}, domain.Conflict(codeAPIKeyHashExists,
					"an api key with this hash already exists").WithCause(err)
			}
			return auth.Key{}, domain.Conflict(codeAPIKeyExists,
				"api key %s already exists", k.ID).WithCause(err)
		}
		return auth.Key{}, err
	}
	return k, nil
}

// ByHash returns the record whose hash equals hash. No row is a domain
// KindNotFound, which is a storage miss and not an authentication decision: the
// service turns a miss into its own ErrUnauthenticated, so this store never
// returns that error and never decides that a caller is unauthenticated.
//
// The lookup is one indexed read on the unique hash column, and the hash is the
// only thing it matches on, so an unknown key finds nothing rather than
// something that partly resembles it.
func (r *Repository) ByHash(ctx context.Context, hash []byte) (auth.Key, error) {
	const q = `select id, name, prefix, hash, created_at from api_keys where hash = $1`

	var k auth.Key
	err := r.pool.QueryRow(ctx, q, hash).Scan(&k.ID, &k.Name, &k.Prefix, &k.Hash, &k.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Key{}, domain.NotFound(codeAPIKeyNotFound, "no api key with this hash")
	}
	if err != nil {
		return auth.Key{}, err
	}
	return k, nil
}
