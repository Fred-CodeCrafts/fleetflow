// Package sim is a small mesoscopic traffic simulator: it moves a fleet of
// vehicles over the road graph, lets fleet vehicles ask an engine for routes and
// report the speeds they experience, and collects the numbers used to compare a
// dynamic run against a static baseline.
package sim

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"

	"fleetflow/internal/graph"
)

// Demand is one trip request: a vehicle that leaves From for To at DepartS
// simulated seconds.
type Demand struct {
	Index     int
	VehicleID string
	From      string
	To        string
	DepartS   float64
	// Fleet is true when the vehicle asks the routing service for routes and
	// sends speed reports, instead of driving a fixed free-flow route.
	Fleet bool
}

// DemandConfig controls demand generation.
type DemandConfig struct {
	// Vehicles is how many trips to generate.
	Vehicles int
	// FleetShare is the fraction of vehicles that use the routing service, in
	// the range [0, 1]. The first round(Vehicles * FleetShare) vehicles are fleet.
	FleetShare float64
	// WindowS spreads departure times uniformly over [0, WindowS) seconds.
	WindowS float64
}

// GenerateDemand builds a deterministic set of trips from the graph's nodes. The
// same seed and config always produce the same demand, which is what makes a
// baseline run and a FleetFlow run comparable.
//
// A graph with fewer than two nodes cannot produce any origin-destination pair,
// so it yields no demand.
func GenerateDemand(g *graph.Graph, seed int64, cfg DemandConfig) []Demand {
	if cfg.Vehicles <= 0 {
		return nil
	}

	nodeIDs := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	if len(nodeIDs) < 2 {
		return nil
	}

	// PCG is a specified generator, so a seed gives the same stream on every
	// platform and Go release that implements it.
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b97f4a7c15))

	fleetCount := int(math.Round(float64(cfg.Vehicles) * clamp01(cfg.FleetShare)))

	demand := make([]Demand, 0, cfg.Vehicles)
	for i := 0; i < cfg.Vehicles; i++ {
		from := nodeIDs[rng.IntN(len(nodeIDs))]
		to := from
		for to == from {
			to = nodeIDs[rng.IntN(len(nodeIDs))]
		}

		demand = append(demand, Demand{
			Index:     i,
			VehicleID: fmt.Sprintf("v%04d", i),
			From:      from,
			To:        to,
			DepartS:   rng.Float64() * cfg.WindowS,
			Fleet:     i < fleetCount,
		})
	}
	return demand
}

// clamp01 limits v to the closed interval [0, 1].
func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
