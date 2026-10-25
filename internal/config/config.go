// Package config loads FleetFlow's settings from the environment and validates
// them at startup, so a bad deployment fails fast with a clear message instead of
// misbehaving later.
//
// Every setting comes from the environment (PRD US-8): no flags, no config file,
// which is what Cloud Run hands a container.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Defaults used when the matching environment variable is unset.
const (
	DefaultPort     = 8080
	DefaultEnv      = "development"
	DefaultOSRMURL  = "http://127.0.0.1:5000"
	DefaultDBPort   = "5432"
	DefaultLogLevel = "info"
)

// Env values accepted by Load.
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

// Log levels accepted by LOG_LEVEL, lowest first. The names are the ones slog
// prints, so an operator reading a log line can set the level by copying the word
// they see.
const (
	LogDebug = "debug"
	LogInfo  = "info"
	LogWarn  = "warn"
	LogError = "error"
)

// Config is the validated runtime configuration.
type Config struct {
	// Addr is the address the HTTP server listens on, for example ":8080".
	Addr string
	// Env is the deployment environment, EnvDevelopment or EnvProduction.
	Env string
	// OSRMURL is the base URL of the OSRM routing service.
	OSRMURL string
	// DatabaseDSN is the configured connection string, or "" when none is set.
	// Fleet records use it; the routing hot path never does (PRD US-5).
	DatabaseDSN string
	// DatabaseAddr is the host:port used for readiness checks, or "" when no
	// database is configured.
	DatabaseAddr string
	// LogLevel is the lowest level the process writes: LogDebug, LogInfo, LogWarn
	// or LogError. Request lines are Info and per-tick detail is Debug, so a
	// production deployment can keep the first and drop the second without a
	// rebuild.
	LogLevel string
	// SumoRuntime contains the configured SUMO tool paths. An empty network
	// disables scenario execution without affecting OSRM or real GPS.
	SumoRuntime SumoRuntime
	// RequireOperatorKey makes the routes that change fleet data demand an API
	// key. It is off by default so a demonstration console works without one,
	// and /readyz reports which mode the process is in, so an open booth is
	// visible rather than assumed.
	RequireOperatorKey bool
}

type SumoRuntime struct {
	Home        string
	Network     string
	Python      string
	RandomTrips string
	DuaRouter   string
	Binary      string
	WorkDir     string
}

// Load reads configuration from getenv (normally os.Getenv) and validates it.
// Any error names the offending variable and the accepted values.
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, errors.New("getenv function is required")
	}

	cfg := Config{
		Env:     valueOr(getenv, "ENV", EnvDevelopment),
		OSRMURL: valueOr(getenv, "OSRM_URL", DefaultOSRMURL),
	}
	if err := validateOSRMURL(cfg.OSRMURL); err != nil {
		return Config{}, err
	}

	level := strings.ToLower(valueOr(getenv, "LOG_LEVEL", DefaultLogLevel))
	switch level {
	case LogDebug, LogInfo, LogWarn, LogError:
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL %q: must be one of debug, info, warn, error", level)
	}

	cfg.LogLevel = level

	switch strings.ToLower(valueOr(getenv, "REQUIRE_OPERATOR_KEY", "false")) {
	case "", "false", "0", "no":
	case "true", "1", "yes":
		cfg.RequireOperatorKey = true
	default:
		return Config{}, fmt.Errorf("REQUIRE_OPERATOR_KEY %q: must be true or false",
			getenv("REQUIRE_OPERATOR_KEY"))
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

	sumoRuntime, err := loadSumoRuntime(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.SumoRuntime = sumoRuntime

	return cfg, nil
}

func loadSumoRuntime(getenv func(string) string) (SumoRuntime, error) {
	home := strings.TrimSpace(getenv("SUMO_HOME"))
	network := strings.TrimSpace(getenv("SUMO_NETWORK"))
	if network == "" {
		return SumoRuntime{Home: home}, nil
	}

	extension := ""
	if runtime.GOOS == "windows" {
		extension = ".exe"
	}
	binary := strings.TrimSpace(getenv("SUMO_BINARY"))
	duarouter := strings.TrimSpace(getenv("SUMO_DUAROUTER"))
	randomTrips := strings.TrimSpace(getenv("SUMO_RANDOM_TRIPS"))
	if home != "" {
		if binary == "" {
			binary = filepath.Join(home, "bin", "sumo"+extension)
		}
		if duarouter == "" {
			duarouter = filepath.Join(home, "bin", "duarouter"+extension)
		}
		if randomTrips == "" {
			randomTrips = filepath.Join(home, "tools", "randomTrips.py")
		}
	}
	if binary == "" {
		return SumoRuntime{}, fmt.Errorf("SUMO_BINARY or SUMO_HOME is required when SUMO_NETWORK is set")
	}
	if duarouter == "" {
		return SumoRuntime{}, fmt.Errorf("SUMO_DUAROUTER or SUMO_HOME is required when SUMO_NETWORK is set")
	}
	if randomTrips == "" {
		return SumoRuntime{}, fmt.Errorf("SUMO_RANDOM_TRIPS or SUMO_HOME is required when SUMO_NETWORK is set")
	}

	python := strings.TrimSpace(getenv("SUMO_PYTHON"))
	if python == "" {
		python = "python3"
		if runtime.GOOS == "windows" {
			python = "python"
		}
	}
	return SumoRuntime{
		Home:        home,
		Network:     network,
		Python:      python,
		RandomTrips: randomTrips,
		DuaRouter:   duarouter,
		Binary:      binary,
		WorkDir:     strings.TrimSpace(getenv("SUMO_WORK_DIR")),
	}, nil
}

func validateOSRMURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("OSRM_URL %q: must be an http(s) base URL without credentials, query, or fragment", value)
	}
	return nil
}

// database resolves the optional database settings. DATABASE_URL wins; otherwise
// the discrete POSTGRES_* variables are used, so the same values can drive the
// service and the local docker compose database.
func database(getenv func(string) string) (dsn, addr string, err error) {
	if raw := strings.TrimSpace(getenv("DATABASE_URL")); raw != "" {
		u, err := url.Parse(raw)
		if err != nil {
			// The value is deliberately not echoed: a connection string carries a
			// password, and a startup log is not the place for it.
			return "", "", errors.New("DATABASE_URL: not a valid URL")
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

	dsn, err = discreteDSN(getenv, host, port)
	if err != nil {
		return "", "", err
	}
	return dsn, net.JoinHostPort(host, port), nil
}

// discreteDSN assembles a connection string from the POSTGRES_* variables — the
// same ones the local docker compose database reads. Those variables alone give a
// host and port, which is enough to open a TCP connection but not to
// authenticate, so the credentials are folded in here.
//
// url.URL escapes every part, so a password containing "@" or ":" survives. An
// empty result means the user did not configure credentials, which is not an
// error: DATABASE_URL or the driver's own environment may supply them.
func discreteDSN(getenv func(string) string, host, port string) (string, error) {
	user := strings.TrimSpace(getenv("POSTGRES_USER"))
	database := strings.TrimSpace(getenv("POSTGRES_DATABASE"))
	password := getenv("POSTGRES_PASSWORD")

	if password != "" && user == "" {
		// Refusing beats silently dropping the password and then failing to
		// authenticate with a message that does not mention the real problem.
		return "", errors.New("POSTGRES_PASSWORD: set without POSTGRES_USER")
	}
	if user == "" {
		return "", nil
	}

	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, port)}
	if database != "" {
		u.Path = "/" + database
	}
	if password != "" {
		u.User = url.UserPassword(user, password)
	} else {
		u.User = url.User(user)
	}
	return u.String(), nil
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
