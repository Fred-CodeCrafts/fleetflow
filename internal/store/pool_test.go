package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpen_RequiresAConnectionString(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("Open() error = nil, want an error when no connection string is given")
	}
}

// TestOpen_DoesNotEchoTheConnectionString guards the startup log. A connection
// string carries a password, and a failed parse is exactly when it would be
// tempting to include it.
func TestOpen_DoesNotEchoTheConnectionString(t *testing.T) {
	_, err := Open(context.Background(), "postgres://user:secret%zz@127.0.0.1/fleetflow")
	if err == nil {
		t.Fatal("Open() error = nil, want an error for an invalid connection string")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("error = %q, which leaks the password", err)
	}
}

// TestOpen_ConnectsToADatabase is the integration test. It is skipped unless a
// database is configured, so the unit suite needs no server; T39 requires the
// integration tests to run against a real Postgres, which is what this covers.
func TestOpen_ConnectsToADatabase(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run the database integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	defer pool.Close()

	// A trivial query proves the connection can actually be used, not just
	// opened: a pooled connection that is never exercised is not evidence.
	var one int
	if err := pool.QueryRow(ctx, "select 1").Scan(&one); err != nil {
		t.Fatalf("select 1: %v", err)
	}
	if one != 1 {
		t.Errorf("select 1 returned %d", one)
	}
}
