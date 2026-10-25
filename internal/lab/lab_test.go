package lab

import (
	"math"
	"reflect"
	"testing"
)

func TestBuildGridShape(t *testing.T) {
	n := DefaultNetwork()
	g, err := Build(n)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got, want := len(g.Nodes), n.Size*n.Size; got != want {
		t.Fatalf("nodes = %d, want %d", got, want)
	}
	// Every lattice point has a two-way street to its right and below neighbour.
	wantEdges := 2 * 2 * n.Size * (n.Size - 1)
	if got := len(g.Edges); got != wantEdges {
		t.Fatalf("edges = %d, want %d", got, wantEdges)
	}

	// A street on an arterial row is the arterial: shorter and faster.
	arterial, _, ok := g.EdgeByID("e_n_0_0_n_0_1")
	if !ok {
		t.Fatal("arterial edge e_n_0_0_n_0_1 missing")
	}
	if arterial.LengthM != n.ArterialLengthM || arterial.SpeedLimitMPS != n.ArterialSpeedMPS {
		t.Fatalf("arterial = %g m at %g m/s, want %g m at %g m/s",
			arterial.LengthM, arterial.SpeedLimitMPS, n.ArterialLengthM, n.ArterialSpeedMPS)
	}
	side, _, ok := g.EdgeByID("e_n_1_0_n_1_1")
	if !ok {
		t.Fatal("side street e_n_1_0_n_1_1 missing")
	}
	if side.LengthM != n.SpacingM || side.SpeedLimitMPS != n.SpeedMPS {
		t.Fatalf("side street = %g m at %g m/s, want %g m at %g m/s",
			side.LengthM, side.SpeedLimitMPS, n.SpacingM, n.SpeedMPS)
	}
}

func TestBuildRejectsBadNetwork(t *testing.T) {
	for _, n := range []Network{
		{Size: 1, SpacingM: 500, SpeedMPS: 8},
		{Size: 6, SpacingM: 0, SpeedMPS: 8},
		{Size: 6, SpacingM: 500, SpeedMPS: 0},
		{Size: 6, SpacingM: 500, SpeedMPS: 8, ArterialEvery: 3},
	} {
		if _, err := Build(n); err == nil {
			t.Fatalf("Build(%+v) accepted an invalid network", n)
		}
	}
}

func TestRunIsDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	first, err := Run(cfg)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := Run(cfg)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same seed gave different results:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

// The adoption control: with nobody routing on the metric, both arms drive the
// same paths, so the comparison must be exactly zero rather than merely small.
func TestZeroShareTiesTheBaseline(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Share = 0
	res, err := Run(cfg)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.DynamicMeanS != res.StaticMeanS {
		t.Fatalf("dynamic %.6f s != static %.6f s at zero adoption", res.DynamicMeanS, res.StaticMeanS)
	}
	if res.ImprovementPct != 0 {
		t.Fatalf("improvement = %g, want 0", res.ImprovementPct)
	}
	if res.DynamicRouteChanges != 0 {
		t.Fatalf("route changes = %d, want 0", res.DynamicRouteChanges)
	}
}

func TestDynamicImprovesOnTheDefaultExperiment(t *testing.T) {
	cfg := DefaultConfig()
	summary, err := Sweep(cfg, 5)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if summary.ImprovementPct <= 2 {
		t.Fatalf("mean improvement = %.2f%%, want more than 2%% on the default experiment", summary.ImprovementPct)
	}
	if summary.ImprovementMinPct <= 0 {
		t.Fatalf("worst seed = %.2f%%, want every seed to improve", summary.ImprovementMinPct)
	}
	if summary.ImprovementMinPct > summary.ImprovementPct || summary.ImprovementPct > summary.ImprovementMaxPct {
		t.Fatalf("bounds do not contain the mean: min %.2f mean %.2f max %.2f",
			summary.ImprovementMinPct, summary.ImprovementPct, summary.ImprovementMaxPct)
	}
}

func TestSweepSeedsAreTheBaseSeedPlusIndex(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 40
	summary, err := Sweep(cfg, 3)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	for i, res := range summary.Runs {
		if want := int64(40 + i); res.Seed != want {
			t.Fatalf("run %d seed = %d, want %d", i, res.Seed, want)
		}
	}
}

func TestRunCarriesTheSeries(t *testing.T) {
	cfg := DefaultConfig()
	res, err := Run(cfg)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.Series) != cfg.Waves {
		t.Fatalf("series has %d waves, want %d", len(res.Series), cfg.Waves)
	}
	for i, wave := range res.Series {
		if wave.Index != i {
			t.Fatalf("wave %d has index %d", i, wave.Index)
		}
		if wave.StaticMeanS <= 0 || wave.DynamicMeanS <= 0 {
			t.Fatalf("wave %d has a non-positive mean: %+v", i, wave)
		}
	}
}

func TestInvalidConfigIsRejected(t *testing.T) {
	base := DefaultConfig()
	cases := map[string]func(*Config){
		"zero waves":      func(c *Config) { c.Waves = 0 },
		"zero vehicles":   func(c *Config) { c.Vehicles = 0 },
		"share above one": func(c *Config) { c.Share = 1.5 },
		"negative share":  func(c *Config) { c.Share = -0.1 },
		"nan share":       func(c *Config) { c.Share = math.NaN() },
	}
	for name, mutate := range cases {
		cfg := base
		mutate(&cfg)
		if _, err := Run(cfg); err == nil {
			t.Fatalf("%s: Run accepted an invalid configuration", name)
		}
	}
}
