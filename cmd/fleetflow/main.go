// Command fleetflow runs the FleetFlow routing service and dashboard.
//
// All settings come from the environment (see internal/config); a bad value
// stops the process before it serves anything.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fleetflow/internal/api"
	"fleetflow/internal/auth"
	authmemory "fleetflow/internal/auth/memory"
	authpostgres "fleetflow/internal/auth/postgres"
	"fleetflow/internal/config"
	"fleetflow/internal/driver"
	drivermemory "fleetflow/internal/driver/memory"
	driverpostgres "fleetflow/internal/driver/postgres"
	"fleetflow/internal/logging"
	"fleetflow/internal/osrm"
	"fleetflow/internal/simulation"
	simulationmemory "fleetflow/internal/simulation/memory"
	simulationpostgres "fleetflow/internal/simulation/postgres"
	"fleetflow/internal/simulation/sumo"
	"fleetflow/internal/store"
	"fleetflow/internal/vehicle"
	vehiclememory "fleetflow/internal/vehicle/memory"
	vehiclepostgres "fleetflow/internal/vehicle/postgres"
)

// issueKeyTimeout bounds the database work behind -issue-key, so an operator
// command that cannot reach the database fails rather than hangs.
const issueKeyTimeout = 10 * time.Second

// issueAPIKey stores a new API key and prints it once, then returns so the process
// exits without serving.
//
// There is no endpoint that mints keys. Minting a credential is an operator
// action, not an API feature, and a route for it would need the very
// authentication it is handing out. A database is required: a key that vanishes at
// the next restart is worse than no key, because it fails later and less clearly.
//
// The plaintext goes to stdout and nowhere else. That one line is the only moment
// the key exists in readable form — the store keeps only its digest — so it must
// not pass through the logger.
func issueAPIKey(logger *slog.Logger, dsn, name string) {
	if dsn == "" {
		fatal(logger, "issuing an API key needs a database: a key that vanishes at restart is worse than none")
	}

	ctx, cancel := context.WithTimeout(context.Background(), issueKeyTimeout)
	defer cancel()

	pool, err := store.Open(ctx, dsn)
	if err != nil {
		fatal(logger, "database configured but unusable", slog.String("error", err.Error()))
	}
	defer pool.Close()

	plaintext, record, err := auth.NewService(authpostgres.New(pool)).Issue(ctx, auth.IssueInput{Name: name})
	if err != nil {
		fatal(logger, "could not issue an api key", slog.String("error", err.Error()))
	}

	fmt.Println(plaintext)
	logger.Info("issued api key",
		slog.String("name", record.Name),
		slog.String("prefix", record.Prefix),
		slog.String("id", record.ID))
}

func main() {
	issueKeyName := flag.String("issue-key", "", "issue an API key with this name, print it once, and exit")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		// Configuration is read before the logger is built, so this one line is
		// written by the default logger: the level is one of the things that
		// could not be read.
		slog.Error("invalid configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger := logging.New(os.Stderr, logging.Options{
		Level:  cfg.LogLevel,
		Format: logging.FormatFor(cfg.Env),
		Env:    cfg.Env,
	})
	slog.SetDefault(logger)

	if *issueKeyName != "" {
		issueAPIKey(logger, cfg.DatabaseDSN, *issueKeyName)
		return
	}

	router, err := osrm.New(cfg.OSRMURL)
	if err != nil {
		fatal(logger, "invalid OSRM configuration", slog.String("error", err.Error()))
	}

	// SIGINT and SIGTERM both mean "stop accepting work and drain".
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := []api.Option{
		api.WithRouter(router),
		api.WithLogger(logger),
	}
	var simulationRepo simulation.Repository

	if cfg.DatabaseDSN != "" {
		// A database that is configured but unreachable stops the process: a
		// wrong host or credential is a deployment mistake, and starting anyway
		// would only turn it into failed requests later. Readiness covers the
		// database going away after a healthy start.
		pool, err := store.Open(ctx, cfg.DatabaseDSN)
		if err != nil {
			fatal(logger, "database configured but unusable", slog.String("error", err.Error()))
		}
		defer pool.Close()

		keys := auth.NewService(authpostgres.New(pool))
		simulationRepo = simulationpostgres.New(pool)

		// The record endpoints read and write through these. The pool is shared,
		// so the process still owns exactly one.
		opts = append(opts,
			api.WithDatabaseCheck(pool.Ping),
			api.WithVehicles(vehicle.NewService(vehiclepostgres.New(pool))),
			api.WithDrivers(driver.NewService(driverpostgres.New(pool))),
			api.WithAPIKeys(keys),
		)
		logger.Info("fleet records and api keys in postgres",
			slog.String("database", cfg.DatabaseAddr))
	} else {
		// Without a database the record endpoints still serve, from memory. That
		// is what lets the service run locally without Postgres, and it is said
		// out loud: nothing here survives a restart, and the deployed
		// configuration always has a database.
		keys := authmemory.New()
		simulationRepo = simulationmemory.New()
		opts = append(opts,
			api.WithVehicles(vehicle.NewService(vehiclememory.New())),
			api.WithDrivers(driver.NewService(drivermemory.New())),
			api.WithAPIKeys(auth.NewService(keys)),
		)
		logger.Warn("no database configured: fleet records and api keys are in memory and will be lost on restart")
	}

	if cfg.SumoRuntime.Network != "" {
		runtime := cfg.SumoRuntime
		runner, err := sumo.NewRunner(ctx, sumo.Toolchain{
			Python:      runtime.Python,
			RandomTrips: runtime.RandomTrips,
			DuaRouter:   runtime.DuaRouter,
			SUMO:        runtime.Binary,
			Network:     runtime.Network,
			WorkDir:     runtime.WorkDir,
		})
		if err != nil {
			fatal(logger, "SUMO runtime configured but unusable", slog.String("error", err.Error()))
		}
		service, err := simulation.NewService(ctx, simulationRepo, runner, logger)
		if err != nil {
			fatal(logger, "could not initialize SUMO simulation service", slog.String("error", err.Error()))
		}
		opts = append(opts, api.WithSimulation(service))
		logger.Info("SUMO scenario execution is configured",
			slog.String("network", runtime.Network),
			slog.String("binary", runtime.Binary))
	} else {
		logger.Warn("SUMO scenarios are disabled: SUMO_NETWORK is not set")
	}

	logger.Info("listening", slog.String("addr", cfg.Addr))
	if err := api.NewServer(opts...).Run(ctx, cfg.Addr); err != nil {
		fatal(logger, "server stopped", slog.String("error", err.Error()))
	}
}

// fatal logs at error level and exits non-zero.
//
// slog has no Fatal, deliberately: a library must not decide to end the process.
// This program does decide, at startup, where a failure to configure routing or bind
// the port is not something to carry on from — and it should still say why in the
// same format as every other line, which is what os.Exit after the log gives.
func fatal(logger *slog.Logger, msg string, attrs ...slog.Attr) {
	logger.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
	os.Exit(1)
}
