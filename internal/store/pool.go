// Package store owns FleetFlow's PostgreSQL connection and schema. Fleet records
// (vehicles, drivers, trips, saved runs and hashed API keys) live here and
// survive a restart; live congestion state does not, and the routing hot path
// never touches this package (PRD US-5, DESIGN 3.7).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool settings.
//
// The service runs as one instance (DESIGN 3.8), so the pool only has to cover
// the API's own concurrency rather than a fleet of replicas. Connections are
// recycled so a long-lived process does not hold sockets open across a database
// restart or a network change.
const (
	maxConns        = 10
	minConns        = 1
	maxConnLifetime = time.Hour
	maxConnIdleTime = 30 * time.Minute
	connectTimeout  = 5 * time.Second
)

// Open connects to PostgreSQL and proves the connection works before returning.
//
// It pings on purpose: a pool that cannot reach its database is worse than no
// pool, because readiness would report ready and then every request would fail.
// Callers should treat the error as fatal at startup, when a wrong credential or
// host should be loud, while readiness covers the database going away later.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, errors.New("a database connection string is required")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// The underlying error can quote the connection string, which carries a
		// password, so it is not wrapped into a message anyone might log.
		return nil, errors.New("the database connection string is not valid")
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime

	// A request that cannot reach the database must fail rather than pile up
	// waiting for a socket that may never open.
	cfg.ConnConfig.ConnectTimeout = connectTimeout

	// NewWithConfig is lazy: it parses and prepares the pool without connecting.
	// The ping below is what actually opens a connection.
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating the database pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("the database is not reachable: %w", err)
	}
	return pool, nil
}
