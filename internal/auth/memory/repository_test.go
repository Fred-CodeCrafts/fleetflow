package memory_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"fleetflow/internal/auth"
	"fleetflow/internal/auth/memory"
	"fleetflow/internal/domain"
)

// record returns a key record whose hash is the real hash of key, so the store
// is exercised the way the service uses it.
func record(id, key string) auth.Key {
	return auth.Key{
		ID:        id,
		Name:      "test " + id,
		Prefix:    auth.DisplayPrefix(key),
		Hash:      auth.Hash(key),
		CreatedAt: time.Unix(0, 0).UTC(),
	}
}

func TestCreateThenByHash(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	want := record("k1", "flf_secret-one")
	stored, err := repo.Create(ctx, want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored.ID != want.ID || !bytes.Equal(stored.Hash, want.Hash) {
		t.Errorf("Create returned %+v, want %+v", stored, want)
	}

	got, err := repo.ByHash(ctx, auth.Hash("flf_secret-one"))
	if err != nil {
		t.Fatalf("ByHash: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ByHash returned %+v, want the stored record", got)
	}
}

func TestCreateRejectsDuplicateID(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	if _, err := repo.Create(ctx, record("k1", "flf_secret-one")); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := repo.Create(ctx, record("k1", "flf_secret-two"))
	if err == nil {
		t.Fatal("Create accepted a duplicate id")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
}

func TestCreateRejectsDuplicateHash(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	if _, err := repo.Create(ctx, record("k1", "flf_secret-one")); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	// A different id but the same key is the same secret, so the unique hash a
	// Postgres schema will carry rejects it.
	_, err := repo.Create(ctx, record("k2", "flf_secret-one"))
	if err == nil {
		t.Fatal("Create accepted a duplicate hash")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
}

func TestCreateRejectsMissingIDOrHash(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	if _, err := repo.Create(ctx, auth.Key{Hash: auth.Hash("flf_x")}); err == nil {
		t.Error("Create accepted a record with no id")
	} else if domain.KindOf(err) != domain.KindInternal {
		t.Errorf("Create kind = %s, want internal", domain.KindOf(err))
	}

	if _, err := repo.Create(ctx, auth.Key{ID: "k1"}); err == nil {
		t.Error("Create accepted a record with no hash")
	} else if domain.KindOf(err) != domain.KindInternal {
		t.Errorf("Create kind = %s, want internal", domain.KindOf(err))
	}
}

func TestByHashUnknownIsNotFound(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	if _, err := repo.Create(ctx, record("k1", "flf_secret-one")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.ByHash(ctx, auth.Hash("flf_not-stored"))
	if err == nil {
		t.Fatalf("ByHash returned %+v for an unknown hash", got)
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("ByHash kind = %s, want not_found", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "api_key_not_found" {
		t.Errorf("ByHash code = %q (ok=%v), want api_key_not_found", code, ok)
	}
}

func TestListReturnsStoredRecordsInCreationOrder(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	for _, id := range []string{"k1", "k2", "k3"} {
		if _, err := repo.Create(ctx, record(id, "flf_secret-"+id)); err != nil {
			t.Fatalf("Create(%s): %v", id, err)
		}
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List returned %d records, want 3", len(list))
	}
	for i, want := range []string{"k1", "k2", "k3"} {
		if list[i].ID != want {
			t.Errorf("list[%d].ID = %q, want %q", i, list[i].ID, want)
		}
	}
}

func TestRecordsAreCopiedInAndOut(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	in := record("k1", "flf_secret-one")
	hash := in.Hash
	if _, err := repo.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Mutating the caller's slice after Create must not change the store.
	hash[0] ^= 0xff
	got, err := repo.ByHash(ctx, auth.Hash("flf_secret-one"))
	if err != nil {
		t.Fatalf("ByHash: %v", err)
	}

	// Mutating the returned slice must not change the store either.
	got.Hash[0] ^= 0xff
	again, err := repo.ByHash(ctx, auth.Hash("flf_secret-one"))
	if err != nil {
		t.Fatalf("ByHash: %v", err)
	}
	if !bytes.Equal(again.Hash, auth.Hash("flf_secret-one")) {
		t.Error("the store changed after a caller mutated a returned hash")
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	list[0].Hash[0] ^= 0xff
	again, err = repo.ByHash(ctx, auth.Hash("flf_secret-one"))
	if err != nil {
		t.Fatalf("ByHash: %v", err)
	}
	if !bytes.Equal(again.Hash, auth.Hash("flf_secret-one")) {
		t.Error("the store changed after a caller mutated a listed hash")
	}
}

func TestMethodsHonourACancelledContext(t *testing.T) {
	repo := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := repo.Create(ctx, record("k1", "flf_secret-one")); !errors.Is(err, context.Canceled) {
		t.Errorf("Create error = %v, want context.Canceled", err)
	}
	if _, err := repo.ByHash(ctx, auth.Hash("flf_secret-one")); !errors.Is(err, context.Canceled) {
		t.Errorf("ByHash error = %v, want context.Canceled", err)
	}
	if _, err := repo.List(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("List error = %v, want context.Canceled", err)
	}
}
