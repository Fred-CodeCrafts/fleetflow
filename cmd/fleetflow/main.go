// Command fleetflow runs the FleetFlow routing service and dashboard.
package main

import (
	"flag"
	"log"
	"os"

	"fleetflow/internal/api"
	"fleetflow/internal/graph"
)

func main() {
	graphPath := flag.String("graph", "data/graph.json", "path to the road graph JSON file")
	addr := flag.String("addr", defaultAddr(), "address to listen on")
	flag.Parse()

	g, err := graph.LoadFromFile(*graphPath)
	if err != nil {
		log.Fatalf("failed to load graph from %s: %v", *graphPath, err)
	}
	log.Printf("FleetFlow loaded %d nodes and %d edges from %s", len(g.Nodes), len(g.Edges), *graphPath)

	if err := api.NewServer(g).ListenAndServe(*addr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// defaultAddr returns the address to listen on. Cloud Run sets PORT, so honour
// it when it is present.
func defaultAddr() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}
