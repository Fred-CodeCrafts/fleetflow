package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/domain"
	"fleetflow/internal/run"
	"fleetflow/internal/run/postgres"
	"fleetflow/internal/store/storetest"
)

// newRepository returns a Postgres repository over the test database and empties
// the fleet tables first, so each test chooses the state it starts from. It
// skips, through storetest.Pool, unless TEST_DATABASE_URL is set.
func newRepository(t *testing.T) (*postgres.Repository, *pgxpool.Pool) {
	t.Helper()
	pool := storetest.Pool(t)
	storetest.Reset(t, pool)
	return postgres.New(pool), pool
}

// record returns a storable run. CreatedAt is left zero on purpose: the database
// sets it, which is what Create must read back.
func record(id string, seed int64) run.Run {
	return run.Run{
		ID:      id,
		Seed:    seed,
		Config:  []byte(`{"fleet_share":0.4}`),
		Metrics: []byte(`{"baseline":{"trip_time":10}}`),
	}
}

// mustCreate stores r or fails the test.
func mustCreate(t *testing.T, repo *postgres.Repository, r run.Run) run.Run {
	t.Helper()
	stored, err := repo.Create(context.Background(), r)
	if err != nil {
		t.Fatalf("Create(%s): %v", r.ID, err)
	}
	return stored
}

// sameJSON reports whether two JSON documents hold the same value. A jsonb column
// canonicalizes what it stores, so two documents that differ only in whitespace or
// key order are the same document as far as the API and its callers are concerned.
func sameJSON(t *testing.T, got, want []byte) bool {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("decoding %s: %v", got, err)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("decoding %s: %v", want, err)
	}
	return reflect.DeepEqual(g, w)
}

func TestCreateThenGetRoundTrips(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	want := record("run_1", 42)
	stored, err := repo.Create(ctx, want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored.CreatedAt.IsZero() {
		t.Error("Create returned a zero created_at; the store did not read the database's own value back")
	}

	got, err := repo.Get(ctx, "run_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != want.ID || got.Seed != want.Seed {
		t.Errorf("Get returned %+v, want the stored run %+v", got, want)
	}
	// The documents have to survive the trip through jsonb unchanged in meaning,
	// which is what the API relies on. jsonb is canonical, so the bytes may differ
	// from what was written by whitespace or key order; the comparison is on the
	// decoded value, which is the promise a JSON column makes.
	if !sameJSON(t, got.Config, want.Config) {
		t.Errorf("Get config = %s, want %s", got.Config, want.Config)
	}
	if !sameJSON(t, got.Metrics, want.Metrics) {
		t.Errorf("Get metrics = %s, want %s", got.Metrics, want.Metrics)
	}
	if !got.CreatedAt.Equal(stored.CreatedAt) {
		t.Errorf("Get created_at = %s, want the stored %s", got.CreatedAt, stored.CreatedAt)
	}
}

// TestCreateKeepsTheSeedUnjudged checks that the column really is a plain bigint:
// zero and negative seeds are stored and read back as they were given.
func TestCreateKeepsTheSeedUnjudged(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	seeds := map[string]int64{"run_a": 0, "run_b": -7, "run_c": 1 << 40}
	for id, seed := range seeds {
		mustCreate(t, repo, record(id, seed))
	}
	for id, seed := range seeds {
		got, err := repo.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get(%s): %v", id, err)
		}
		if got.Seed != seed {
			t.Errorf("Get(%s) seed = %d, want %d", id, got.Seed, seed)
		}
	}
}

// TestListOrdersMostRecentFirst pins the contract: List is most recent first, with
// the id as the tiebreaker for rows that share a timestamp.
func TestListOrdersMostRecentFirst(t *testing.T) {
	repo, pool := newRepository(t)
	ctx := context.Background()

	for _, id := range []string{"run_1", "run_2", "run_3"} {
		mustCreate(t, repo, record(id, 1))
	}
	// The three inserts can share an instant, so the order is made explicit: the
	// timestamps are set an increasing number of seconds apart, which is the case
	// the descending index serves. The offset is interpolated as a literal because
	// it is a constant this test owns, not input.
	for i, id := range []string{"run_1", "run_2", "run_3"} {
		q := fmt.Sprintf("update runs set created_at = now() + interval '%d seconds' where id = $1", i)
		if _, err := pool.Exec(ctx, q, id); err != nil {
			t.Fatalf("stamping %s: %v", id, err)
		}
	}

	list, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List returned %d runs, want 3", len(list))
	}
	if list[0].ID != "run_3" || list[1].ID != "run_2" || list[2].ID != "run_1" {
		t.Errorf("List order = %s, %s, %s, want run_3, run_2, run_1",
			list[0].ID, list[1].ID, list[2].ID)
	}
}

func TestListHonoursTheLimit(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	for _, id := range []string{"run_1", "run_2", "run_3"} {
		mustCreate(t, repo, record(id, 1))
	}

	list, err := repo.List(ctx, 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List(2) returned %d runs, want 2", len(list))
	}
}

// TestSecondRepositoryReadsWhatTheFirstWrote is the "survives a restart"
// requirement: the run lives in the database, not in a repository instance, so a
// fresh Repository over the same pool reads it back.
func TestSecondRepositoryReadsWhatTheFirstWrote(t *testing.T) {
	_, pool := newRepository(t)
	ctx := context.Background()

	mustCreate(t, postgres.New(pool), record("run_1", 5))

	second := postgres.New(pool)
	got, err := second.Get(ctx, "run_1")
	if err != nil {
		t.Fatalf("Get from the second repository: %v", err)
	}
	if got.ID != "run_1" || got.Seed != 5 {
		t.Errorf("second repository read %+v, want run_1 with seed 5", got)
	}
}

func TestCreateDuplicateIDIsAConflict(t *testing.T) {
	repo, _ := newRepository(t)
	ctx := context.Background()

	mustCreate(t, repo, record("run_1", 1))

	_, err := repo.Create(ctx, record("run_1", 2))
	if err == nil {
		t.Fatal("Create accepted a duplicate id")
	}
	if kind := domain.KindOf(err); kind != domain.KindConflict {
		t.Errorf("Create kind = %s, want conflict", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "run_exists" {
		t.Errorf("Create error code = %q (ok = %v), want run_exists", code, ok)
	}
}

func TestGetUnknownIsNotFound(t *testing.T) {
	repo, _ := newRepository(t)

	_, err := repo.Get(context.Background(), "run_missing")
	if err == nil {
		t.Fatal("Get returned an unknown run")
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Get kind = %s, want not_found", kind)
	}
	if code, _, ok := domain.Public(err); !ok || code != "run_not_found" {
		t.Errorf("Get error code = %q (ok = %v), want run_not_found", code, ok)
	}
}
