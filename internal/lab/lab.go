package lab

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"fleetflow/internal/engine"
	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
)

const (
	// congestionPerVehicle is how much one more simultaneous vehicle on an edge
	// slows it down, in the speed-flow curve below.
	congestionPerVehicle = 0.25
	// maxCongestionFactor caps that slowdown at a quarter of free-flow speed.
	maxCongestionFactor = 4.0
	// perceptionNoise is how much a driver's private view of an edge's cost may
	// differ from the published metric, as a fraction. Without it every informed
	// vehicle reads the same numbers and they all divert onto the same street at
	// the same time, which is a herding artefact of the model rather than a
	// property of the idea.
	perceptionNoise = 0.35
	// waveDuration is how much simulated time passes between two waves, and so
	// between two publications of the metric.
	waveDuration = 10 * time.Minute
	// minTripHops keeps sampled trips long enough to have a choice of routes.
	minTripHops = 3
)

// ErrInvalidConfig is returned when the lab cannot run the configuration asked
// for.
var ErrInvalidConfig = errors.New("lab: invalid configuration")

// Config describes one experiment. Every number that changes the result is here,
// so a run can be repeated from its configuration alone.
type Config struct {
	// Network is the lattice the experiment runs on.
	Network Network
	// Waves is how many batches of demand are released, one per metric
	// customization.
	Waves int
	// Vehicles is how many trips each wave releases.
	Vehicles int
	// Share is the fraction of trips that route on the dynamic metric. The rest
	// drive the free-flow route whatever the congestion reports say, which is how
	// adoption rate enters the comparison.
	Share float64
	// Seed makes the demand and the adoption draws reproducible.
	Seed int64
	// Params overrides the dynamic cost model. Nil means weights.DefaultParams.
	Params *weights.Params
}

// DefaultConfig is the experiment the console runs when nobody chooses one.
func DefaultConfig() Config {
	return Config{
		Network:  DefaultNetwork(),
		Waves:    12,
		Vehicles: 60,
		Share:    0.5,
		Seed:     1,
	}
}

func (c Config) validate() error {
	if err := c.Network.validate(); err != nil {
		return err
	}
	switch {
	case c.Waves < 1 || c.Waves > 200:
		return fmt.Errorf("%w: waves must be between 1 and 200, got %d", ErrInvalidConfig, c.Waves)
	case c.Vehicles < 1 || c.Vehicles > 2000:
		return fmt.Errorf("%w: vehicles per wave must be between 1 and 2000, got %d", ErrInvalidConfig, c.Vehicles)
	case math.IsNaN(c.Share) || c.Share < 0 || c.Share > 1:
		return fmt.Errorf("%w: share must be in [0, 1], got %g", ErrInvalidConfig, c.Share)
	}
	return nil
}

// Wave is one batch of demand and what it cost each arm.
type Wave struct {
	Index        int     `json:"index"`
	StaticMeanS  float64 `json:"static_mean_s"`
	DynamicMeanS float64 `json:"dynamic_mean_s"`
	Reports      int     `json:"reports"`
}

// Result is one seed run: the two arms on identical demand.
type Result struct {
	Seed                int64   `json:"seed"`
	Share               float64 `json:"share"`
	StaticMeanS         float64 `json:"static_mean_s"`
	DynamicMeanS        float64 `json:"dynamic_mean_s"`
	StaticMaxS          float64 `json:"static_max_s"`
	DynamicMaxS         float64 `json:"dynamic_max_s"`
	ImprovementPct      float64 `json:"improvement_pct"`
	DynamicRouteChanges int     `json:"dynamic_route_changes"`
	Series              []Wave  `json:"series"`
}

// Summary is a set of seeds at one adoption share.
type Summary struct {
	Runs           []Result `json:"runs"`
	Share          float64  `json:"share"`
	StaticMeanS    float64  `json:"static_mean_s"`
	DynamicMeanS   float64  `json:"dynamic_mean_s"`
	ImprovementPct float64  `json:"improvement_pct"`
	// ImprovementMinPct and ImprovementMaxPct bound the per-seed results, so a
	// reader can see whether the effect is consistent or one seed carrying the
	// average.
	ImprovementMinPct float64 `json:"improvement_min_pct"`
	ImprovementMaxPct float64 `json:"improvement_max_pct"`
}

type trip struct {
	from, to string
	informed bool
}

// Run executes one configuration and returns the comparison for one seed.
func Run(cfg Config) (Result, error) {
	if err := cfg.validate(); err != nil {
		return Result{}, err
	}
	g, err := Build(cfg.Network)
	if err != nil {
		return Result{}, err
	}
	router := engine.NewDijkstraRouter(g)
	base, err := engine.NewBaseline(g, router)
	if err != nil {
		return Result{}, fmt.Errorf("lab: baseline engine: %w", err)
	}
	params := weights.DefaultParams()
	if cfg.Params != nil {
		params = *cfg.Params
	}
	dyn, err := engine.New(g, router, params)
	if err != nil {
		return Result{}, fmt.Errorf("lab: dynamic engine: %w", err)
	}

	waves := demand(cfg)
	res := Result{Seed: cfg.Seed, Share: cfg.Share, Series: make([]Wave, 0, len(waves))}
	clock := time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)

	var staticTotal, dynamicTotal float64
	var tripCount int

	for i, wave := range waves {
		staticPaths := make([][]int, len(wave))
		dynamicPaths := make([][]int, len(wave))
		metric := dyn.ActiveMetric()

		for j, t := range wave {
			p, err := base.Route(t.from, t.to)
			if err != nil {
				return Result{}, fmt.Errorf("lab: baseline route %s->%s: %w", t.from, t.to, err)
			}
			staticPaths[j] = p.Edges

			if !t.informed {
				dynamicPaths[j] = p.Edges
				continue
			}
			// The driver reads the published metric through a private error, so
			// informed vehicles spread over alternatives instead of all taking
			// the same one. The draw is seeded, so the run stays reproducible.
			view := withPerceptionNoise(g, metric, cfg.Seed, i, j)
			q, err := engine.NewDijkstraRouter(g).Route(view, t.from, t.to)
			if err != nil {
				return Result{}, fmt.Errorf("lab: dynamic route %s->%s: %w", t.from, t.to, err)
			}
			dynamicPaths[j] = q.Edges
			if !samePath(p.Edges, q.Edges) {
				res.DynamicRouteChanges++
			}
		}

		staticTimes := travelTimes(g, staticPaths)
		dynamicTimes := travelTimes(g, dynamicPaths)

		var staticSum, dynamicSum float64
		for _, v := range staticTimes {
			staticSum += v
		}
		for _, v := range dynamicTimes {
			dynamicSum += v
		}
		staticTotal += staticSum
		dynamicTotal += dynamicSum
		tripCount += len(wave)

		maxOf := func(v []float64) float64 {
			var m float64
			for _, x := range v {
				m = math.Max(m, x)
			}
			return m
		}
		res.StaticMaxS = math.Max(res.StaticMaxS, maxOf(staticTimes))
		res.DynamicMaxS = math.Max(res.DynamicMaxS, maxOf(dynamicTimes))

		reports := 0
		dynamicLoad := loadByEdge(g, dynamicPaths)
		// Every fleet vehicle is a probe: it reports the speed it managed on each
		// edge it used, whether or not it is one that routes on the metric. That
		// is what a fleet telematics feed gives the service in practice.
		for j := range wave {
			for _, e := range dynamicPaths[j] {
				if err := dyn.Report(g.Edges[e].ID, g.Edges[e].SpeedLimitMPS/slowdown(dynamicLoad[e]), clock); err != nil {
					return Result{}, fmt.Errorf("lab: report on %s: %w", g.Edges[e].ID, err)
				}
				reports++
			}
		}
		dyn.Customize()

		res.Series = append(res.Series, Wave{
			Index:        i,
			StaticMeanS:  staticSum / float64(len(wave)),
			DynamicMeanS: dynamicSum / float64(len(wave)),
			Reports:      reports,
		})
		clock = clock.Add(waveDuration)
	}

	res.StaticMeanS = staticTotal / float64(tripCount)
	res.DynamicMeanS = dynamicTotal / float64(tripCount)
	if res.StaticMeanS > 0 {
		res.ImprovementPct = (res.StaticMeanS - res.DynamicMeanS) / res.StaticMeanS * 100
	}
	return res, nil
}

// Sweep runs the configuration once per seed and aggregates the comparison.
//
// Seeds are the configuration's seed plus 0..n-1, so the sweep is as reproducible
// as a single run and the console can offer "ten seeds" without naming them.
func Sweep(cfg Config, seeds int) (Summary, error) {
	if seeds < 1 || seeds > 100 {
		return Summary{}, fmt.Errorf("%w: seeds must be between 1 and 100, got %d", ErrInvalidConfig, seeds)
	}
	out := Summary{Share: cfg.Share, Runs: make([]Result, 0, seeds)}
	var staticTotal, dynamicTotal float64
	out.ImprovementMinPct = math.Inf(1)
	out.ImprovementMaxPct = math.Inf(-1)

	for i := 0; i < seeds; i++ {
		one := cfg
		one.Seed = cfg.Seed + int64(i)
		res, err := Run(one)
		if err != nil {
			return Summary{}, err
		}
		out.Runs = append(out.Runs, res)
		staticTotal += res.StaticMeanS
		dynamicTotal += res.DynamicMeanS
		out.ImprovementMinPct = math.Min(out.ImprovementMinPct, res.ImprovementPct)
		out.ImprovementMaxPct = math.Max(out.ImprovementMaxPct, res.ImprovementPct)
	}
	out.StaticMeanS = staticTotal / float64(seeds)
	out.DynamicMeanS = dynamicTotal / float64(seeds)
	if out.StaticMeanS > 0 {
		out.ImprovementPct = (out.StaticMeanS - out.DynamicMeanS) / out.StaticMeanS * 100
	}
	return out, nil
}

// demand samples every wave's trips once, so both arms drive exactly the same
// origin-destination pairs in the same order.
func demand(cfg Config) [][]trip {
	rng := rand.New(rand.NewSource(cfg.Seed))
	waves := make([][]trip, cfg.Waves)
	for w := range waves {
		wave := make([]trip, cfg.Vehicles)
		for v := range wave {
			from := sampleNode(rng, cfg.Network.Size)
			to := sampleNode(rng, cfg.Network.Size)
			for sameNode(from, to) || gridDistance(from, to) < minTripHops {
				to = sampleNode(rng, cfg.Network.Size)
			}
			wave[v] = trip{from: from, to: to, informed: rng.Float64() < cfg.Share}
		}
		waves[w] = wave
	}
	return waves
}

func sampleNode(rng *rand.Rand, size int) string {
	return nodeID(rng.Intn(size), rng.Intn(size))
}

func sameNode(a, b string) bool { return a == b }

// gridDistance is the Manhattan distance between two node IDs, and the only
// reason the ID format is parseable.
func gridDistance(a, b string) int {
	var r1, c1, r2, c2 int
	if _, err := fmt.Sscanf(a, "n_%d_%d", &r1, &c1); err != nil {
		return 0
	}
	if _, err := fmt.Sscanf(b, "n_%d_%d", &r2, &c2); err != nil {
		return 0
	}
	return abs(r1-r2) + abs(c1-c2)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// slowdown is the speed-flow curve: the first vehicle on an edge travels at the
// limit, and every further simultaneous vehicle adds congestionPerVehicle to the
// travel time, capped so the edge never falls below a quarter of free flow.
func slowdown(vehicles int) float64 {
	if vehicles <= 1 {
		return 1
	}
	return math.Min(1+congestionPerVehicle*float64(vehicles-1), maxCongestionFactor)
}

func loadByEdge(g *graph.Graph, paths [][]int) []int {
	load := make([]int, len(g.Edges))
	for _, path := range paths {
		for _, e := range path {
			load[e]++
		}
	}
	return load
}

// travelTimes prices each trip against the congestion the whole wave creates, so
// every trip in a wave experiences the same load rather than the order of
// departure deciding who pays.
func travelTimes(g *graph.Graph, paths [][]int) []float64 {
	load := loadByEdge(g, paths)
	times := make([]float64, len(paths))
	for i, path := range paths {
		var total float64
		for _, e := range path {
			total += g.Edges[e].FreeFlowS * slowdown(load[e])
		}
		times[i] = total
	}
	return times
}

// withPerceptionNoise returns the published metric as one driver sees it: each
// edge costs what the metric says, up to a private error of perceptionNoise.
//
// This is a standard driver-perception term, not a trick to improve the result:
// a metric published to a thousand vehicles that all read it identically would
// send them all down the same street, and the congestion they create is an
// artefact of the model, not a finding about the idea. The error is drawn from
// the run's seed, wave and vehicle index, so two runs of the same seed make the
// same choices.
func withPerceptionNoise(g *graph.Graph, m *engine.Metric, seed int64, wave, vehicle int) *engine.Metric {
	if m == nil {
		return engine.NewStaticMetric(g)
	}
	rng := rand.New(rand.NewSource(seed*1_000_003 + int64(wave)*1_003 + int64(vehicle)))
	costs := make([]float64, m.Len())
	for i := range costs {
		costs[i] = m.Cost(i) * (1 + perceptionNoise*rng.Float64())
	}
	return engine.NewMetricFor(g, costs, m.Version())
}

func samePath(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
