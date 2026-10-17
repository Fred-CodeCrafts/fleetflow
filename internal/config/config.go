// Package config loads FleetFlow's settings from the environment and validates
// them at startup, so a bad deployment fails fast with a clear message instead of
// misbehaving later.
//
// Every setting comes from the environment (PRD US-8): no flags, no config file,
// which is what Cloud Run hands a container.
package config

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Defaults used when the matching environment variable is unset.
const (
	DefaultPort      = 8080
	DefaultEnv       = "development"
	DefaultGraphPath = "data/graph.json"
	DefaultDBPort    = "5432"
)

// Env values accepted by Load.
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

// Config is the validated runtime configuration.
type Config struct {
	// Addr is the address the HTTP server listens on, for example ":8080".
	Addr string
	// Env is the deployment environment, EnvDevelopment or EnvProduction.
	Env string
	// GraphPath is the road graph file to load at startup.
	GraphPath string
	// DatabaseDSN is the configured connection string, or "" when none is set.
	// Fleet records use it; the routing hot path never does (PRD US-5).
	DatabaseDSN string
	// DatabaseAddr is the host:port used for readiness checks, or "" when no
	// database is configured.
	DatabaseAddr string
}

// Load reads configuration from getenv (normally os.Getenv) and validates it.
// Any error names the offending variable and the accepted values.
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, errors.New("getenv function is required")
	}

	cfg := Config{
		Env:       valueOr(getenv, "ENV", EnvDevelopment),
		GraphPath: valueOr(getenv, "GRAPH_PATH", DefaultGraphPath),
	}

	port := valueOr(getenv, "PORT", strconv.Itoa(DefaultPort))
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Config{}, fmt.Errorf("PORT %q: must be an integer between 1 and 65535", port)
	}
	cfg.Addr = ":" + strconv.Itoa(n)

	switch cfg.Env {
	case EnvDevelopment, EnvProduction:
	default:
		return Config{}, fmt.Errorf("ENV %q: must be %q or %q", cfg.Env, EnvDevelopment, EnvProduction)
	}

	dsn, addr, err := database(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.DatabaseDSN, cfg.DatabaseAddr = dsn, addr

	return cfg, nil
}

// database resolves the optional database settings. DATABASE_URL wins; otherwise
// the discrete POSTGRES_* variables are used, so the same values can drive the
// service and the local docker compose database.
func database(getenv func(string) string) (dsn, addr string, err error) {
	if raw := strings.TrimSpace(getenv("DATABASE_URL")); raw != "" {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", fmt.Errorf("DATABASE_URL: not a valid URL: %v", err)
		}
		switch u.Scheme {
		case "postgres", "postgresql":
		default:
			return "", "", fmt.Errorf("DATABASE_URL: scheme %q is not supported, want \"postgres\"", u.Scheme)
		}
		if u.Hostname() == "" {
			return "", "", errors.New("DATABASE_URL: missing host")
		}
		return raw, net.JoinHostPort(u.Hostname(), portOr(u.Port())), nil
	}

	host := strings.TrimSpace(getenv("POSTGRES_HOST"))
	if host == "" {
		return "", "", nil
	}
	port := valueOr(getenv, "POSTGRES_PORT", DefaultDBPort)
	if _, err := strconv.Atoi(port); err != nil {
		return "", "", fmt.Errorf("POSTGRES_PORT %q: must be an integer", port)
	}
	return "", net.JoinHostPort(host, port), nil
}

// TCPReachable returns a readiness probe that reports whether addr accepts a TCP
// connection. It checks the network path to the database, not credentials or
// schema; a SQL-level check replaces it once the database layer exists.
func TCPReachable(addr string) func(context.Context) error {
	return func(ctx context.Context) error {
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return err
		}
		return conn.Close()
	}
}

// valueOr returns the trimmed value of key, or fallback when it is unset or
// empty.
func valueOr(getenv func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return fallback
}

// portOr returns port, or the default database port when it is empty.
func portOr(port string) string {
	if port == "" {
		return DefaultDBPort
	}
	return port
}
