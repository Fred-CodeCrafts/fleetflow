// Package storetest is the plumbing the database integration tests share.
//
// Unit tests never touch a database (DESIGN 3.7), so these helpers skip unless
// TEST_DATABASE_URL names one. They are not a test framework: they open a pool
// against the schema Flyway has already applied and empty the fleet tables
// between tests, so an integration test can choose the state it starts from.
//
// Reset empties tables that the whole schema shares, so two packages must not use
// it against the same database at the same time. `go test ./...` runs packages in
// parallel: give each package its own database, or run them one at a time with
// `-p 1`. Otherwise one package truncates another's rows mid-test and the failure
// looks like a race in the code rather than in the harness.
package storetest

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/store"
)

// fleetTables are emptied by Reset, youngest first. They are listed explicitly
// rather than discovered, so a new table has to be added here on purpose instead
// of being quietly left out of a cleanup.
var fleetTables = []string{"trips", "drivers", "vehicles"}

// Pool returns a pool to the database named by TEST_DATABASE_URL, and skips the
// test when that variable is unset, which is what keeps the unit suite free of a
// server.
//
// When the variable is set but the schema is missing, the test fails with the
// command that applies it: finding out from a confusing "relation does not
// exist" error that the migrations were never run is worse.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run the database integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to the test database: %v", err)
	}
	t.Cleanup(pool.Close)

	var found bool
	const q = "select exists (select 1 from pg_tables where schemaname = 'public' and tablename = 'vehicles')"
	if err := pool.QueryRow(ctx, q).Scan(&found); err != nil {
		t.Fatalf("checking the schema: %v", err)
	}
	if !found {
		t.Fatal("the test database has no fleet tables; apply them with " +
			"`docker compose --profile tools run --rm migrate`")
	}
	return pool
}

// Reset empties the fleet tables so each test starts from a state it chose.
func Reset(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, "truncate "+strings.Join(fleetTables, ", ")+" cascade"); err != nil {
		t.Fatalf("emptying the fleet tables: %v", err)
	}
}
