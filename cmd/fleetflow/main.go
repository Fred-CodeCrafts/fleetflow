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
	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
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
	if cfg.DatabaseAddr != "" {
		opts = append(opts, api.WithDatabaseCheck(config.TCPReachable(cfg.DatabaseAddr)))
		log.Printf("database configured at %s", cfg.DatabaseAddr)
	}

	log.Printf("listening on %s (%s)", cfg.Addr, cfg.Env)
	if err := api.NewServer(g, opts...).Run(ctx, cfg.Addr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
