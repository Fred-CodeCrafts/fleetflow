// Command fleetflow runs the FleetFlow routing service and dashboard.
//
// All settings come from the environment (see internal/config); a bad value
// stops the process before it serves anything.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
	"fleetflow/internal/store"
	"fleetflow/internal/trip"
	tripmemory "fleetflow/internal/trip/memory"
	trippostgres "fleetflow/internal/trip/postgres"
	"fleetflow/internal/vehicle"
	vehiclememory "fleetflow/internal/vehicle/memory"
	vehiclepostgres "fleetflow/internal/vehicle/postgres"
	"fleetflow/internal/weights"
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
func issueAPIKey(dsn, name string) {
	if dsn == "" {
		log.Fatal("issuing an API key needs a database: a key that vanishes at restart is worse than none")
	}

	ctx, cancel := context.WithTimeout(context.Background(), issueKeyTimeout)
	defer cancel()

	pool, err := store.Open(ctx, dsn)
	if err != nil {
		log.Fatalf("database configured but unusable: %v", err)
	}
	defer pool.Close()

	plaintext, record, err := auth.NewService(authpostgres.New(pool)).Issue(ctx, auth.IssueInput{Name: name})
	if err != nil {
		log.Fatalf("could not issue an api key: %v", err)
	}

	fmt.Println(plaintext)
	log.Printf("issued api key %q (prefix %s, id %s)", record.Name, record.Prefix, record.ID)
}

func main() {
	issueKeyName := flag.String("issue-key", "", "issue an API key with this name, print it once, and exit")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	if *issueKeyName != "" {
		issueAPIKey(cfg.DatabaseDSN, *issueKeyName)
		return
	}

	g, err := graph.LoadFromFile(cfg.GraphPath)
	if err != nil {
		log.Fatalf("failed to load graph from %s: %v", cfg.GraphPath, err)
	}
	log.Printf("FleetFlow loaded %d nodes and %d edges from %s", len(g.Nodes), len(g.Edges), cfg.GraphPath)

	eng, err := engine.New(g, engine.NewDijkstraRouter(g), weights.DefaultParams())
	if err != nil {
		log.Fatalf("failed to build the routing engine: %v", err)
	}
	log.Printf("customizing weights every %s", eng.CustomizeEvery())

	// SIGINT and SIGTERM both mean "stop accepting work and drain".
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The customizer rebuilds the metric from the reports that have arrived, so
	// the weights a route query reads keep up with the traffic. It keeps its own
	// context so shutdown can stop it before the listener drains, rather than
	// after main returns.
	customizerCtx, stopCustomizer := context.WithCancel(context.Background())
	defer stopCustomizer()
	go eng.Run(customizerCtx)

	opts := []api.Option{
		api.WithEngine(eng),
		api.WithStopFunc(func(context.Context) error {
			stopCustomizer()
			return nil
		}),
	}

	if cfg.DatabaseDSN != "" {
		// A database that is configured but unreachable stops the process: a
		// wrong host or credential is a deployment mistake, and starting anyway
		// would only turn it into failed requests later. Readiness covers the
		// database going away after a healthy start.
		pool, err := store.Open(ctx, cfg.DatabaseDSN)
		if err != nil {
			log.Fatalf("database configured but unusable: %v", err)
		}
		defer pool.Close()

		// The record endpoints read and write through these. The pool is shared,
		// so the process still owns exactly one.
		opts = append(opts,
			api.WithDatabaseCheck(pool.Ping),
			api.WithVehicles(vehicle.NewService(vehiclepostgres.New(pool))),
			api.WithDrivers(driver.NewService(driverpostgres.New(pool))),
			api.WithTrips(trip.NewService(trippostgres.New(pool))),
			api.WithAPIKeys(auth.NewService(authpostgres.New(pool))),
		)
		log.Printf("fleet records and api keys in postgres at %s", cfg.DatabaseAddr)
	} else {
		// Without a database the record endpoints still serve, from memory. That
		// is what lets the service run locally without Postgres, and it is said
		// out loud: nothing here survives a restart, and the deployed
		// configuration always has a database.
		opts = append(opts,
			api.WithVehicles(vehicle.NewService(vehiclememory.New())),
			api.WithDrivers(driver.NewService(drivermemory.New())),
			api.WithTrips(trip.NewService(tripmemory.New())),
			api.WithAPIKeys(auth.NewService(authmemory.New())),
		)
		log.Print("no database configured: fleet records and api keys are in memory and will be lost on restart")
	}

	log.Printf("listening on %s (%s)", cfg.Addr, cfg.Env)
	if err := api.NewServer(g, opts...).Run(ctx, cfg.Addr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
