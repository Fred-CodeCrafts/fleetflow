// Package lab runs a seeded, closed-loop comparison between static and dynamic
// edge weights on a synthetic grid.
//
// It exists to make the routing idea from the author's thesis proposal visible
// and re-runnable: the dynamic engine keeps one congestion state per edge, learns
// it from the speed reports the simulated vehicles make, and routes around the
// edges it has learned to avoid. The same demand, on the same grid, is also routed
// by a static engine that always believes the speed limit, which is the baseline
// the dynamic arm is compared against.
//
// What this is not: it is not the thesis, and it is not a traffic simulation of
// Jakarta. There is no CCH, no TraCI loop and no LuST scenario here, and the
// network is a lattice. The congestion model is a deliberately simple
// speed-flow curve, so the numbers describe this model, not a real city.
package lab

import (
	"fmt"
	"math"

	"fleetflow/internal/graph"
)

// metresPerDegreeLat is the length of one degree of latitude at the equator,
// close enough over the few kilometres a grid spans.
const metresPerDegreeLat = 111_320.0

// Network describes the lattice the experiment runs on.
//
// The arterials matter to the comparison: a network of identical streets gives
// the router no reason to prefer one street over another, so a static route
// spreads itself and there is nothing for dynamic weights to improve. Every
// Nth row and column is therefore faster and shorter, the way a city's main
// roads are, and the baseline concentrates on them until they congest.
type Network struct {
	// Size is the number of nodes along one side.
	Size int
	// SpacingM is the length of an ordinary street, in metres.
	SpacingM float64
	// SpeedMPS is the free-flow speed of an ordinary street.
	SpeedMPS float64
	// ArterialEvery marks every Nth row and column as an arterial; 0 disables
	// arterials and leaves a uniform lattice.
	ArterialEvery int
	// ArterialLengthM and ArterialSpeedMPS are the arterial's length and limit.
	ArterialLengthM  float64
	ArterialSpeedMPS float64
}

// DefaultNetwork is the lattice the console shows.
func DefaultNetwork() Network {
	return Network{
		Size:             6,
		SpacingM:         500,
		SpeedMPS:         8.33, // 30 km/h on a side street
		ArterialEvery:    3,    // rows and columns 0 and 3
		ArterialLengthM:  400,  // 20% shorter than a side street
		ArterialSpeedMPS: 12.5, // 45 km/h
	}
}

func (n Network) validate() error {
	switch {
	case n.Size < 2 || n.Size > 20:
		return fmt.Errorf("%w: grid size must be between 2 and 20, got %d", ErrInvalidConfig, n.Size)
	case math.IsNaN(n.SpacingM) || n.SpacingM <= 0:
		return fmt.Errorf("%w: street length must be positive, got %g", ErrInvalidConfig, n.SpacingM)
	case math.IsNaN(n.SpeedMPS) || n.SpeedMPS <= 0:
		return fmt.Errorf("%w: street speed limit must be positive, got %g", ErrInvalidConfig, n.SpeedMPS)
	case n.ArterialEvery < 0:
		return fmt.Errorf("%w: arterial interval must not be negative, got %d", ErrInvalidConfig, n.ArterialEvery)
	case n.ArterialEvery > 0 && (n.ArterialLengthM <= 0 || n.ArterialSpeedMPS <= 0):
		return fmt.Errorf("%w: arterials need a positive length and speed limit", ErrInvalidConfig)
	}
	return nil
}

// Build returns the network's graph.
//
// It is built in code rather than loaded from a file so that a clone can
// reproduce the experiment with nothing but the repository: the grid the lab runs
// on cannot go missing, and its identity is its size and its arterials.
func Build(n Network) (*graph.Graph, error) {
	if err := n.validate(); err != nil {
		return nil, err
	}

	g := graph.New()
	// Jakarta, so the map and the numbers are about the city this project serves,
	// even though the lattice is not its street plan.
	const originLat, originLon = -6.2040, 106.8160
	dLat := n.SpacingM / metresPerDegreeLat
	dLon := n.SpacingM / (metresPerDegreeLat * math.Cos(originLat*math.Pi/180))

	for r := 0; r < n.Size; r++ {
		for c := 0; c < n.Size; c++ {
			node := graph.Node{
				ID:  nodeID(r, c),
				Lat: originLat + float64(r)*dLat,
				Lon: originLon + float64(c)*dLon,
			}
			if err := g.AddNode(node); err != nil {
				return nil, fmt.Errorf("lab: add node %s: %w", node.ID, err)
			}
		}
	}

	for r := 0; r < n.Size; r++ {
		for c := 0; c < n.Size; c++ {
			if c+1 < n.Size {
				if err := addPair(g, n, r, c, r, c+1, r); err != nil {
					return nil, err
				}
			}
			if r+1 < n.Size {
				if err := addPair(g, n, r, c, r+1, c, c); err != nil {
					return nil, err
				}
			}
		}
	}
	return g, nil
}

// addPair adds both directions of one street. Two-way edges are what let the
// router prefer the opposite side of a congested segment, which is half of what
// the lab is showing.
//
// arterialIndex is the row of a horizontal street or the column of a vertical
// one: a street lying on an arterial row or column gets the arterial values.
func addPair(g *graph.Graph, n Network, r1, c1, r2, c2, arterialIndex int) error {
	length, speed := n.SpacingM, n.SpeedMPS
	if n.ArterialEvery > 0 && arterialIndex%n.ArterialEvery == 0 {
		length, speed = n.ArterialLengthM, n.ArterialSpeedMPS
	}

	from, to := nodeID(r1, c1), nodeID(r2, c2)
	for _, pair := range [][2]string{{from, to}, {to, from}} {
		edge := graph.Edge{
			ID:            "e_" + pair[0] + "_" + pair[1],
			From:          pair[0],
			To:            pair[1],
			LengthM:       length,
			SpeedLimitMPS: speed,
			Lanes:         2,
		}
		if err := g.AddEdge(edge); err != nil {
			return fmt.Errorf("lab: add edge %s: %w", edge.ID, err)
		}
	}
	return nil
}

func nodeID(row, col int) string {
	return fmt.Sprintf("n_%d_%d", row, col)
}
