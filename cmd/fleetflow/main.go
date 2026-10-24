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
	"fleetflow/internal/dispatch"
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
	"fleetflow/internal/trip"
	tripmemory "fleetflow/internal/trip/memory"
	trippostgres "fleetflow/internal/trip/postgres"
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
		api.WithRouter(osrm.NewFallbackRouter(router)),
		api.WithLogger(logger),
	}
	var simulationRepo simulation.Repository
	// Both branches keep the fleet services in locals: the API options and the
	// scenario dispatcher must be the same instances, or a dispatched vehicle
	// would not appear in the register the console reads.
	var fleetSvc *vehicle.Service
	var tripSvc *trip.Service

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

		fleetSvc = vehicle.NewService(vehiclepostgres.New(pool))
		tripSvc = trip.NewService(trippostgres.New(pool))

		// The record endpoints read and write through these. The pool is shared,
		// so the process still owns exactly one.
		opts = append(opts,
			api.WithDatabaseCheck(pool.Ping),
			api.WithVehicles(fleetSvc),
			api.WithDrivers(driver.NewService(driverpostgres.New(pool))),
			api.WithTrips(tripSvc),
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
		fleetSvc = vehicle.NewService(vehiclememory.New())
		driverSvc := driver.NewService(drivermemory.New())
		tripSvc = trip.NewService(tripmemory.New())
		seedMemoryFleet(ctx, fleetSvc, driverSvc, tripSvc)
		opts = append(opts,
			api.WithVehicles(fleetSvc),
			api.WithDrivers(driverSvc),
			api.WithTrips(tripSvc),
			api.WithAPIKeys(auth.NewService(keys)),
		)
		logger.Warn("no database configured: fleet records and api keys are in memory and will be lost on restart")
	}

	var simService *simulation.Service
	// The scenario dispatcher needs the same fleet stores the API serves from, so
	// they are kept in locals rather than created inline in the options above.
	var simOptions []simulation.Option
	if fleetSvc != nil && tripSvc != nil {
		simOptions = append(simOptions, simulation.WithDispatcher(dispatch.New(fleetSvc, tripSvc)))
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
		simService, err = simulation.NewService(ctx, simulationRepo, runner, logger, simOptions...)
		if err != nil {
			fatal(logger, "could not initialize SUMO simulation service", slog.String("error", err.Error()))
		}
		logger.Info("SUMO scenario execution is configured",
			slog.String("network", runtime.Network),
			slog.String("binary", runtime.Binary))
	} else {
		var err error
		simService, err = simulation.NewService(ctx, simulationRepo, simulation.NewSyntheticRunner(), logger, simOptions...)
		if err != nil {
			logger.Warn("could not initialize simulation service", slog.String("error", err.Error()))
		} else {
			logger.Info("synthetic simulation runner configured with pre-seeded Jakarta logistics scenarios")
		}
	}
	if simService != nil {
		// The three demand modes are seeded here rather than generated when the
		// page opens: the console reads them like any other stored run, and the
		// fleet they describe exists because the dispatch ran once at startup.
		if err := simService.SeedPresetRuns(ctx); err != nil {
			logger.Warn("could not seed the scenario presets", slog.String("error", err.Error()))
		}
		opts = append(opts, api.WithSimulation(simService))
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

func ptrFloat(f float64) *float64 { return &f }
func ptrInt(i int) *int           { return &i }

func seedMemoryFleet(ctx context.Context, vSvc *vehicle.Service, dSvc *driver.Service, tSvc *trip.Service) {
	vehicles := []vehicle.CreateInput{
		{Name: "B-9012-JKT", Type: "truck", Status: "on_trip", Registration: "B-9012-JKT", Make: "Isuzu", Model: "Giga FVM Heavy Cargo", BodyType: "Heavy Cargo Box", Capacity: ptrFloat(14000), Year: ptrInt(2022), Color: "White"},
		{Name: "B-9431-JKT", Type: "truck", Status: "on_trip", Registration: "B-9431-JKT", Make: "Mitsubishi", Model: "Fuso Canter 136 HDL", BodyType: "Box Truck", Capacity: ptrFloat(4500), Year: ptrInt(2023), Color: "Yellow"},
		{Name: "B-9204-JKT", Type: "van", Status: "on_trip", Registration: "B-9204-JKT", Make: "Toyota", Model: "HiAce Commuter Cargo", BodyType: "Van", Capacity: ptrFloat(1200), Year: ptrInt(2023), Color: "Silver"},
		{Name: "B-9550-JKT", Type: "van", Status: "available", Registration: "B-9550-JKT", Make: "Daihatsu", Model: "Gran Max Blind Van", BodyType: "Blind Van", Capacity: ptrFloat(800), Year: ptrInt(2024), Color: "White"},
		{Name: "B-9118-JKT", Type: "motorcycle", Status: "available", Registration: "B-9118-JKT", Make: "Honda", Model: "PCX Electric Cargo", BodyType: "Motorcycle Cargo", Capacity: ptrFloat(60), Year: ptrInt(2024), Color: "Blue"},
		{Name: "B-9672-JKT", Type: "truck", Status: "available", Registration: "B-9672-JKT", Make: "Hino", Model: "Dutro 130 HD Cargo", BodyType: "Box Truck", Capacity: ptrFloat(4500), Year: ptrInt(2023), Color: "Green"},
		{Name: "B-9801-JKT", Type: "van", Status: "available", Registration: "B-9801-JKT", Make: "Wuling", Model: "Formo Blind Van", BodyType: "Blind Van", Capacity: ptrFloat(650), Year: ptrInt(2023), Color: "White"},
	}
	for _, v := range vehicles {
		_, _ = vSvc.Create(ctx, v)
	}

	drivers := []driver.CreateInput{
		{Name: "Budi Santoso", ContactPhone: "+62 812-3456-7890", LicenseClass: "B2 Umum", ShiftStart: "07:00", ShiftEnd: "16:00", Availability: "available", ContractType: "Full-time"},
		{Name: "Agus Setiawan", ContactPhone: "+62 813-9876-5432", LicenseClass: "B1", ShiftStart: "08:00", ShiftEnd: "17:00", Availability: "available", ContractType: "Contract"},
		{Name: "Rian Hidayat", ContactPhone: "+62 815-2233-4455", LicenseClass: "A", ShiftStart: "08:00", ShiftEnd: "17:00", Availability: "available", ContractType: "Full-time"},
		{Name: "Eko Prasetyo", ContactPhone: "+62 817-6677-8899", LicenseClass: "A", ShiftStart: "06:00", ShiftEnd: "15:00", Availability: "available", ContractType: "Contract"},
		{Name: "Dimas Pratama", ContactPhone: "+62 819-3344-5566", LicenseClass: "C", ShiftStart: "09:00", ShiftEnd: "18:00", Availability: "available", ContractType: "Freelance"},
		{Name: "Hendra Gunawan", ContactPhone: "+62 821-4455-6677", LicenseClass: "B1 Umum", ShiftStart: "07:30", ShiftEnd: "16:30", Availability: "available", ContractType: "Full-time"},
		{Name: "Fajar Nugraha", ContactPhone: "+62 856-7788-9900", LicenseClass: "C", ShiftStart: "08:00", ShiftEnd: "17:00", Availability: "available", ContractType: "Mitra"},
	}
	for i, d := range drivers {
		created, err := dSvc.Create(ctx, d)
		if err == nil {
			if i == 0 {
				_, _ = dSvc.Assign(ctx, created.ID, "B-9012-JKT")
			} else if i == 1 {
				_, _ = dSvc.Assign(ctx, created.ID, "B-9431-JKT")
			} else if i == 2 {
				_, _ = dSvc.Assign(ctx, created.ID, "B-9204-JKT")
			}
		}
	}

	trips := []trip.CreateInput{
		// In-progress active dispatches
		{Origin: "Tanjung Priok Port Terminal", Destination: "Kelapa Gading Commercial District", VehicleID: "B-9012-JKT", Status: "in_progress"},
		{Origin: "Pluit Urban Logistics Depot", Destination: "Glodok Commercial Hub", VehicleID: "B-9431-JKT", Status: "in_progress"},
		{Origin: "Halim Air Cargo Terminal", Destination: "Sudirman Business Corridor", VehicleID: "B-9204-JKT", Status: "in_progress"},
		// Planned scheduled dispatches
		{Origin: "Cilandak Urban Fulfillment Hub", Destination: "Senayan City Retail Store", VehicleID: "B-9550-JKT", Status: "planned"},
		{Origin: "Tomang Regional Logistics Spine", Destination: "Grand Indonesia Thamrin", VehicleID: "B-9118-JKT", Status: "planned"},
		{Origin: "Tanjung Priok Port Terminal", Destination: "Soekarno-Hatta Cargo Terminal", VehicleID: "B-9672-JKT", Status: "planned"},
		{Origin: "Halim Air Cargo Terminal", Destination: "Kuningan Logistics Center", VehicleID: "B-9801-JKT", Status: "planned"},
		// Completed past journeys
		{Origin: "Cakung Regional Logistics Depot", Destination: "Pasar Senen Distribution Hub", VehicleID: "B-9431-JKT", Status: "planned"},
		{Origin: "Marunda Bulk Terminal", Destination: "Sunter Industrial Park", VehicleID: "B-9012-JKT", Status: "planned"},
	}
	for i, tr := range trips {
		created, err := tSvc.Create(ctx, tr)
		if err == nil && i >= 7 {
			// Advance to in_progress then completed to record start and finish times
			_, _ = tSvc.SetStatus(ctx, created.ID, "in_progress")
			_, _ = tSvc.SetStatus(ctx, created.ID, "completed")
		}
	}
}
