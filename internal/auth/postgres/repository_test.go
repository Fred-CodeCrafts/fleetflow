package postgres_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/auth"
	"fleetflow/internal/auth/postgres"
	"fleetflow/internal/domain"
	"fleetflow/internal/store/storetest"
)

// newRepository returns a Postgres repository over the test database and empties
// the shared tables first, so each test chooses the state it starts from. It
// skips, through storetest.Pool, unless TEST_DATABASE_URL is set.
func newRepository(t *testing.T) (*postgres.Repository, *pgxpool.Pool) {
	t.Helper()
	pool := storetest.Pool(t)
	storetest.Reset(t, pool)
	return postgres.New(pool), pool
}

// record returns a key record whose hash is the real hash of plaintext, so the
// store is exercised the way the service uses it. CreatedAt is left zero on
// purpose: the database sets it, which is what Create must read back.
func record(id, plaintext string) auth.Key {
	return auth.Key{
		ID:     id,
		Name:   "test " + id,
		Prefix: auth.DisplayPrefix(plaintext),
		Hash:   auth.Hash(plaintext),
	}
}

// mustCreate stores k or fails the test.
func mustCreate(t *testing.T, repo *postgres.Repository, k auth.Key) auth.Key {
	t.Helper()
	stored, err := repo.Create(context.Background(), k)
	if err != nil {
		t.Fatalf("Create(%s): %v", k.ID, err)
	}
	return stored
}

func TestCreateThenByHashRoundTrips(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	stored, err := repo.Create(ctx, record("k1", "flf_secret-one"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored.CreatedAt.IsZero() {
		t.Error("Create returned a zero created_at; the store did not read the database's own value back")
	}

	got, err := repo.ByHash(ctx, auth.Hash("flf_secret-one"))
	if err != nil {
		t.Fatalf("ByHash: %v", err)
	}
	if got.ID != stored.ID || got.Name != stored.Name || got.Prefix != stored.Prefix {
		t.Errorf("ByHash returned %+v, want %+v", got, stored)
	}
	if !bytes.Equal(got.Hash, stored.Hash) {
		t.Errorf("ByHash hash = %x, want %x", got.Hash, stored.Hash)
	}
	if !got.CreatedAt.Equal(stored.CreatedAt) {
		t.Errorf("ByHash created_at = %s, want the stored %s", got.CreatedAt, stored.CreatedAt)
	}
}

// TestSecondRepositoryReadsWhatTheFirstWrote is the "survives a restart"
// requirement: the record lives in the database, not in a repository instance,
// so a fresh Repository over the same pool reads it back.
func TestSecondRepositoryReadsWhatTheFirstWrote(t *testing.T) {
	_, pool := newRepository(t)
	ctx := context.Background()

	mustCreate(t, postgres.New(pool), record("k1", "flf_secret-one"))

	second := postgres.New(pool)
	got, err := second.ByHash(ctx, auth.Hash("flf_secret-one"))
	if err != nil {
		t.Fatalf("ByHash from the second repository: %v", err)
	}
	if got.ID != "k1" {
		t.Errorf("second repository read %+v, want the record k1", got)
	}
}

func TestCreateDuplicateIDIsAConflict(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	mustCreate(t, repo, record("k1", "flf_secret-one"))

	_, err := repo.Create(ctx, record("k1", "flf_secret-two"))
	if err == nil {
		t.Fatal("Create accepted a duplicate id")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "api_key_exists" {
		t.Errorf("Create error code = %q (ok = %v), want api_key_exists", code, ok)
	}
}

func TestCreateDuplicateHashIsAConflict(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	mustCreate(t, repo, record("k1", "flf_secret-one"))

	// A different id but the same key is the same secret, so the unique hash
	// constraint rejects it.
	_, err := repo.Create(ctx, record("k2", "flf_secret-one"))
	if err == nil {
		t.Fatal("Create accepted a duplicate hash")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "api_key_exists" {
		t.Errorf("Create error code = %q (ok = %v), want api_key_exists", code, ok)
	}
}

func TestByHashUnknownIsNotFound(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	mustCreate(t, repo, record("k1", "flf_secret-one"))

	got, err := repo.ByHash(ctx, auth.Hash("flf_not-stored"))
	if err == nil {
		t.Fatalf("ByHash returned %+v for an unknown hash", got)
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("ByHash kind = %s, want not_found", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "api_key_not_found" {
		t.Errorf("ByHash code = %q (ok = %v), want api_key_not_found", code, ok)
	}
}

// TestStoredRecordHoldsOnlyTheDigest pins the core promise: the store keeps the
// SHA-256 of the key and never the plaintext, so a row read back can verify a
// key but cannot reveal it.
func TestStoredRecordHoldsOnlyTheDigest(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	const plaintext = "flf_secret-one"
	stored := mustCreate(t, repo, record("k1", plaintext))

	if !bytes.Equal(stored.Hash, auth.Hash(plaintext)) {
		t.Errorf("stored hash = %x, want the digest %x", stored.Hash, auth.Hash(plaintext))
	}
	if bytes.Equal(stored.Hash, []byte(plaintext)) {
		t.Error("the stored record holds the plaintext key")
	}

	got, err := repo.ByHash(ctx, auth.Hash(plaintext))
	if err != nil {
		t.Fatalf("ByHash: %v", err)
	}
	if bytes.Equal(got.Hash, []byte(plaintext)) {
		t.Error("the record read back holds the plaintext key")
	}
	if !auth.Verify(plaintext, got.Hash) {
		t.Error("the stored digest does not verify the plaintext it was made from")
	}
}
