// Command generate_grid writes the demo road network: a regular grid of streets
// around central Jakarta, in the graph JSON the server loads.
//
// The network's size is not decoration, it sets capacity. Capacity per edge is
// lanes * length_m / 7.5, so 450 m blocks and two lanes hold 16,800 vehicles
// across the graph -- more than any run this simulator can drive, which costs a
// run its congestion and with it any difference between static and dynamic
// routing. The demo graph is therefore generated with both dialled down:
//
//	go run ./scripts/generate_grid.go -spacing-m 150 -lanes 1
//
// That is 20 vehicles per edge, and a peak-hour run congests it. See DESIGN,
// "The demo network", for the measurement behind the choice.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

type Node struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type Edge struct {
	ID            string  `json:"id"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	LengthM       float64 `json:"length_m"`
	SpeedLimitMPS float64 `json:"speed_limit_mps"`
	Lanes         int     `json:"lanes"`
}

type GraphJSON struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

func main() {
	outPath := flag.String("out", "data/graph.json", "Output path for the generated graph JSON")
	rows := flag.Int("rows", 6, "Number of grid rows (North-South)")
	cols := flag.Int("cols", 6, "Number of grid columns (East-West)")
	spacing := flag.Float64("spacing-m", 150, "Meters between grid intersections")
	lanes := flag.Int("lanes", 1, "Lanes on an ordinary street; the central arterials get one more")
	flag.Parse()

	if *rows < 2 || *cols < 2 {
		fmt.Fprintln(os.Stderr, "the grid needs at least 2 rows and 2 columns")
		os.Exit(1)
	}
	if *spacing <= 0 || *lanes < 1 {
		fmt.Fprintln(os.Stderr, "spacing must be positive and there must be at least one lane")
		os.Exit(1)
	}

	// Center around Jakarta (Sudirman / Thamrin area)
	centerLat := -6.2088
	centerLon := 106.8456
	spacingM := *spacing // meters between grid intersections

	// Approximate meters to degrees conversion around Jakarta (-6 deg latitude)
	latDegPerMeter := 1.0 / 111320.0
	lonDegPerMeter := 1.0 / (111320.0 * math.Cos(centerLat*math.Pi/180.0))

	nodes := make([]Node, 0, (*rows)*(*cols))
	edges := make([]Edge, 0)

	nodeID := func(r, c int) string {
		return fmt.Sprintf("N_%d_%d", r, c)
	}

	for r := 0; r < *rows; r++ {
		for c := 0; c < *cols; c++ {
			// Offset from center
			dY := float64(r-(*rows)/2) * spacingM
			dX := float64(c-(*cols)/2) * spacingM

			lat := centerLat + dY*latDegPerMeter
			lon := centerLon + dX*lonDegPerMeter

			nodes = append(nodes, Node{
				ID:  nodeID(r, c),
				Lat: math.Round(lat*1e6) / 1e6,
				Lon: math.Round(lon*1e6) / 1e6,
			})
		}
	}

	midR := (*rows) / 2
	midC := (*cols) / 2

	addEdgePair := func(u, v string, isArterial bool, length float64) {
		streetLanes := *lanes
		speed := 13.89 // ~50 km/h

		if isArterial {
			streetLanes = *lanes + 1
			speed = 16.67 // ~60 km/h
		}

		// Forward edge
		edges = append(edges, Edge{
			ID:            fmt.Sprintf("e_%s_%s", u, v),
			From:          u,
			To:            v,
			LengthM:       length,
			SpeedLimitMPS: speed,
			Lanes:         streetLanes,
		})

		// Reverse edge
		edges = append(edges, Edge{
			ID:            fmt.Sprintf("e_%s_%s", v, u),
			From:          v,
			To:            u,
			LengthM:       length,
			SpeedLimitMPS: speed,
			Lanes:         streetLanes,
		})
	}

	// Add horizontal (East-West) edges
	for r := 0; r < *rows; r++ {
		isArterialRow := (r == midR || r == midR-1)
		for c := 0; c < *cols-1; c++ {
			u := nodeID(r, c)
			v := nodeID(r, c+1)
			addEdgePair(u, v, isArterialRow, spacingM)
		}
	}

	// Add vertical (North-South) edges
	for c := 0; c < *cols; c++ {
		isArterialCol := (c == midC || c == midC-1)
		for r := 0; r < *rows-1; r++ {
			u := nodeID(r, c)
			v := nodeID(r+1, c)
			addEdgePair(u, v, isArterialCol, spacingM)
		}
	}

	graphData := GraphJSON{
		Nodes: nodes,
		Edges: edges,
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create directories: %v\n", err)
		os.Exit(1)
	}

	data, err := json.MarshalIndent(graphData, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to serialize graph: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write graph file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated grid city graph at %s with %d nodes and %d edges\n", *outPath, len(nodes), len(edges))
}
