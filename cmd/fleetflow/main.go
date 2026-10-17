// Command fleetflow runs the FleetFlow routing service and dashboard.
//
// All settings come from the environment (see internal/config); a bad value
// stops the process before it serves anything.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"fleetflow/internal/api"
	"fleetflow/internal/config"
	"fleetflow/internal/graph"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	g, err := graph.LoadFromFile(cfg.GraphPath)
	if err != nil {
		log.Fatalf("failed to load graph from %s: %v", cfg.GraphPath, err)
	}
	log.Printf("FleetFlow loaded %d nodes and %d edges from %s", len(g.Nodes), len(g.Edges), cfg.GraphPath)

	var opts []api.Option
	if cfg.DatabaseAddr != "" {
		opts = append(opts, api.WithDatabaseCheck(config.TCPReachable(cfg.DatabaseAddr)))
		log.Printf("database configured at %s", cfg.DatabaseAddr)
	}

	// SIGINT and SIGTERM both mean "stop accepting work and drain".
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("listening on %s (%s)", cfg.Addr, cfg.Env)
	if err := api.NewServer(g, opts...).Run(ctx, cfg.Addr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
