package sim

import (
	"reflect"
	"testing"

	"fleetflow/internal/graph"
)

func loadFixture(t *testing.T) *graph.Graph {
	t.Helper()
	g, err := graph.LoadFromFile("../../data/test_graph.json")
	if err != nil {
		t.Fatalf("failed to load fixture graph: %v", err)
	}
	return g
}

func TestGenerateDemand_IsDeterministic(t *testing.T) {
	g := loadFixture(t)
	cfg := DemandConfig{Vehicles: 50, FleetShare: 0.5, WindowS: 600}

	first := GenerateDemand(g, 42, cfg)
	second := GenerateDemand(g, 42, cfg)

	if !reflect.DeepEqual(first, second) {
		t.Fatal("the same seed produced different demand")
	}
	if len(first) != 50 {
		t.Fatalf("generated %d trips, want 50", len(first))
	}
}

func TestGenerateDemand_SeedChangesTheDemand(t *testing.T) {
	g := loadFixture(t)
	cfg := DemandConfig{Vehicles: 50, FleetShare: 0.5, WindowS: 600}

	a := GenerateDemand(g, 1, cfg)
	b := GenerateDemand(g, 2, cfg)

	if reflect.DeepEqual(a, b) {
		t.Fatal("two different seeds produced identical demand")
	}
}

func TestGenerateDemand_ValidTrips(t *testing.T) {
	g := loadFixture(t)
	cfg := DemandConfig{Vehicles: 200, FleetShare: 0.25, WindowS: 900}

	for _, d := range GenerateDemand(g, 7, cfg) {
		if d.From == d.To {
			t.Errorf("%s: origin and destination are both %s", d.VehicleID, d.From)
		}
		if _, ok := g.Nodes[d.From]; !ok {
			t.Errorf("%s: unknown origin %s", d.VehicleID, d.From)
		}
		if _, ok := g.Nodes[d.To]; !ok {
			t.Errorf("%s: unknown destination %s", d.VehicleID, d.To)
		}
		if d.DepartS < 0 || d.DepartS >= cfg.WindowS {
			t.Errorf("%s: departure %f outside [0, %f)", d.VehicleID, d.DepartS, cfg.WindowS)
		}
	}
}

func TestGenerateDemand_FleetShare(t *testing.T) {
	g := loadFixture(t)
	demand := GenerateDemand(g, 3, DemandConfig{Vehicles: 200, FleetShare: 0.25, WindowS: 600})

	fleet := 0
	for _, d := range demand {
		if d.Fleet {
			fleet++
		}
	}
	if fleet != 50 {
		t.Errorf("fleet vehicles = %d, want 50", fleet)
	}
}

func TestGenerateDemand_EdgeCases(t *testing.T) {
	g := loadFixture(t)

	if got := GenerateDemand(g, 1, DemandConfig{Vehicles: 0, FleetShare: 1, WindowS: 10}); got != nil {
		t.Errorf("expected no demand for zero vehicles, got %d trips", len(got))
	}

	// Full fleet share and no fleet share.
	all := GenerateDemand(g, 5, DemandConfig{Vehicles: 10, FleetShare: 1, WindowS: 60})
	for _, d := range all {
		if !d.Fleet {
			t.Errorf("%s: expected every vehicle to be fleet at share 1", d.VehicleID)
		}
	}
	none := GenerateDemand(g, 5, DemandConfig{Vehicles: 10, FleetShare: 0, WindowS: 60})
	for _, d := range none {
		if d.Fleet {
			t.Errorf("%s: expected no fleet vehicles at share 0", d.VehicleID)
		}
	}
}

func TestGenerateDemand_SingleNodeGraphHasNoTrips(t *testing.T) {
	g := graph.New()
	if err := g.AddNode(graph.Node{ID: "only"}); err != nil {
		t.Fatalf("unexpected error adding node: %v", err)
	}

	if got := GenerateDemand(g, 1, DemandConfig{Vehicles: 10, FleetShare: 1, WindowS: 10}); got != nil {
		t.Errorf("expected no demand for a one-node graph, got %d trips", len(got))
	}
}
