// Command compare runs the baseline and the FleetFlow arm of the simulator over
// the same seeded demand, for a range of seeds, and reports how the two differ
// (T61, DESIGN 3.4). T60 (parameter tuning) and T62 (fleet-share sweep) read its
// CSV, so its contract is fixed: one row per arm per seed.
//
// It drives sim.RunBoth in-process rather than the public HTTP API. The point of
// a seed sweep is the model, not the transport; the HTTP path is exercised by
// the live simulation and its own tests.
//
// stdout carries the CSV only, so `compare ... > out.csv` produces a clean file.
// The summary of differences goes to stderr, so it can be read without
// polluting the CSV. The same flags always produce a byte-identical CSV, and a
// seed that fails to run is a clear error and a non-zero exit, never a silently
// skipped row.
//
// Flags, with their defaults:
//
//	-graph              data/networks/jakarta.net.xml  network to load
//	-seed-from          1               first seed, inclusive
//	-seed-to            10              last seed, inclusive
//	-vehicles           200             vehicles per run
//	-fleet-share        0.5             share of vehicles using FleetFlow, in [0, 1]
//	-step-s             1               simulated seconds per step
//	-window-s           600             departure window, in simulated seconds
//	-reroute-every-s    30              seconds between reroute checks; 0 disables rerouting
//	-reroute-threshold  0.1             fractional cost improvement a reroute must beat
//	-alpha              1.5             cost penalty strength
//	-beta               2               cost penalty exponent
//	-gamma              0.3             congestion smoothing rate per report, in (0, 1]
//	-lambda             0.1             decay rate per quiet cycle, in [0, 1)
//	-delta              5               customization interval, in seconds
//	-hold               10              grace period after a report before decay starts, in seconds
//	-max-steps          200000          step budget per run, so a stuck run still ends
//	-out                ""              CSV path; empty writes to stdout
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fleetflow/internal/config"
	"fleetflow/internal/graph"
	"fleetflow/internal/sim"
	"fleetflow/internal/weights"
)

// defaultMaxSteps bounds a run that never settles. It matches the server's own
// run budget, and a normal run stops long before it when the demand has settled.
const defaultMaxSteps = 200_000

// arm names as they appear in the CSV and are recognised in the summary.
const (
	armBaseline  = "baseline"
	armFleetFlow = "fleetflow"
)

// csvHeader is the exact column order both consumers (T60, T62) rely on.
const csvHeader = "seed,arm,vehicles,completed_trips,average_trip_s,p95_trip_s," +
	"route_changes,unroutable_vehicles,saturated_edges_pct,utilization_std_dev," +
	"mean_time_lost_s,fleet_trips,mean_fleet_trip_s,background_trips," +
	"mean_background_trip_s"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "compare: %v\n", err)
		os.Exit(1)
	}
}

// options is the resolved command line: one simulation config, one set of
// weights parameters, and the seed range and output path around them.
type options struct {
	graph    string
	seedFrom int64
	seedTo   int64
	cfg      sim.Config
	params   weights.Params
	maxSteps int
	out      string
}

// parseFlags reads args into options, filling defaults and validating the
// result. It returns flag.ErrHelp unchanged when help was asked for.
func parseFlags(args []string, stderr io.Writer) (options, error) {
	demo := sim.DefaultConfig()
	def := weights.DefaultParams()

	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		graphPath = fs.String("graph", config.DefaultGraphPath, "network to load (.net.xml or .json)")
		seedFrom  = fs.Int64("seed-from", 1, "first seed, inclusive")
		seedTo    = fs.Int64("seed-to", 10, "last seed, inclusive")

		vehicles         = fs.Int("vehicles", demo.Vehicles, "vehicles per run")
		fleetShare       = fs.Float64("fleet-share", demo.FleetShare, "share of vehicles using FleetFlow, in [0, 1]")
		stepS            = fs.Float64("step-s", demo.StepS, "simulated seconds per step")
		windowS          = fs.Float64("window-s", demo.WindowS, "departure window, in simulated seconds")
		rerouteEveryS    = fs.Float64("reroute-every-s", demo.RerouteEveryS, "seconds between reroute checks; 0 disables rerouting")
		rerouteThreshold = fs.Float64("reroute-threshold", demo.RerouteThreshold, "fractional cost improvement a reroute must beat")

		alpha  = fs.Float64("alpha", def.Alpha, "cost penalty strength")
		beta   = fs.Float64("beta", def.Beta, "cost penalty exponent")
		gamma  = fs.Float64("gamma", def.Gamma, "congestion smoothing rate per report, in (0, 1]")
		lambda = fs.Float64("lambda", def.Lambda, "decay rate per quiet cycle, in [0, 1)")
		deltaS = fs.Float64("delta", def.Delta.Seconds(), "customization interval, in seconds")
		holdS  = fs.Float64("hold", def.Hold.Seconds(), "grace period after a report before decay starts, in seconds")

		maxSteps = fs.Int("max-steps", defaultMaxSteps, "step budget per run")
		out      = fs.String("out", "", "CSV path; empty writes to stdout")
	)

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	opts := options{
		graph:    *graphPath,
		seedFrom: *seedFrom,
		seedTo:   *seedTo,
		cfg: sim.Config{
			Vehicles:         *vehicles,
			FleetShare:       *fleetShare,
			StepS:            *stepS,
			WindowS:          *windowS,
			RerouteEveryS:    *rerouteEveryS,
			RerouteThreshold: *rerouteThreshold,
		},
		params: weights.Params{
			Alpha:  *alpha,
			Beta:   *beta,
			Gamma:  *gamma,
			Lambda: *lambda,
			Delta:  seconds(*deltaS),
			Hold:   seconds(*holdS),
		},
		maxSteps: *maxSteps,
		out:      *out,
	}
	if err := opts.validate(); err != nil {
		return options{}, err
	}
	return opts, nil
}

// seconds turns a count of seconds into a duration.
func seconds(v float64) time.Duration {
	return time.Duration(v * float64(time.Second))
}

// validate rejects a configuration the simulator would refuse deep inside a
// run, so the caller gets one named flag back instead of an anonymous failure.
func (o options) validate() error {
	switch {
	case o.seedFrom > o.seedTo:
		return fmt.Errorf("seed-from %d is greater than seed-to %d", o.seedFrom, o.seedTo)
	case o.cfg.Vehicles < 0:
		return fmt.Errorf("vehicles must not be negative, got %d", o.cfg.Vehicles)
	case o.cfg.FleetShare < 0 || o.cfg.FleetShare > 1:
		return fmt.Errorf("fleet-share must be in [0, 1], got %g", o.cfg.FleetShare)
	case o.cfg.StepS <= 0:
		return fmt.Errorf("step-s must be positive, got %g", o.cfg.StepS)
	case o.cfg.WindowS < 0:
		return fmt.Errorf("window-s must not be negative, got %g", o.cfg.WindowS)
	case o.cfg.RerouteEveryS < 0:
		return fmt.Errorf("reroute-every-s must not be negative, got %g", o.cfg.RerouteEveryS)
	case o.cfg.RerouteThreshold < 0 || o.cfg.RerouteThreshold >= 1:
		return fmt.Errorf("reroute-threshold must be in [0, 1), got %g", o.cfg.RerouteThreshold)
	case o.maxSteps <= 0:
		return fmt.Errorf("max-steps must be positive, got %d", o.maxSteps)
	}
	if err := o.params.Validate(); err != nil {
		return fmt.Errorf("invalid weights parameters: %w", err)
	}
	return nil
}

// Row is one arm of one seeded run: the seed, which arm, and what it measured.
type Row struct {
	Seed int64
	Arm  string
	M    sim.Metrics
}

// collect runs both arms for every seed in range and returns the rows in a fixed
// order: baseline then FleetFlow, seeds ascending.
func collect(g *graph.Graph, o options) ([]Row, error) {
	rows := make([]Row, 0, int(o.seedTo-o.seedFrom+1)*2)
	for seed := o.seedFrom; seed <= o.seedTo; seed++ {
		cfg := o.cfg
		cfg.Seed = seed

		comparison, err := sim.RunBoth(g, cfg, o.params, o.maxSteps)
		if err != nil {
			return nil, fmt.Errorf("seed %d: %w", seed, err)
		}
		rows = append(rows,
			Row{Seed: seed, Arm: armBaseline, M: comparison.Baseline},
			Row{Seed: seed, Arm: armFleetFlow, M: comparison.FleetFlow},
		)
	}
	return rows, nil
}

// run is the whole tool, taking its streams as arguments so a test can drive it.
func run(args []string, stdout, stderr io.Writer) error {
	opts, err := parseFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	g, err := graph.LoadFromFile(opts.graph)
	if err != nil {
		return fmt.Errorf("load graph %q: %w", opts.graph, err)
	}

	rows, err := collect(g, opts)
	if err != nil {
		return err
	}

	if err := writeOutput(opts.out, rows, stdout); err != nil {
		return err
	}

	writeSummary(stderr, summarize(rows))
	return nil
}

// writeOutput writes the CSV to path, or to stdout when path is empty. The rows
// are complete before anything is written, so a failed run leaves no partial
// CSV behind.
func writeOutput(path string, rows []Row, stdout io.Writer) error {
	if path == "" {
		return writeCSV(stdout, rows)
	}

	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output directory %q: %w", dir, err)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %q: %w", path, err)
	}
	if err := writeCSV(f, rows); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// writeCSV writes the header and one row per arm.
func writeCSV(w io.Writer, rows []Row) error {
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString(csvHeader + "\n"); err != nil {
		return err
	}
	for _, r := range rows {
		if err := writeCSVRow(bw, r); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// writeCSVRow writes one arm's metrics in the header's column order.
func writeCSVRow(w io.Writer, r Row) error {
	m := r.M
	fields := [...]string{
		strconv.FormatInt(r.Seed, 10),
		r.Arm,
		strconv.Itoa(m.TotalVehicles),
		strconv.Itoa(m.CompletedTrips),
		formatFloat(m.AverageTripS),
		formatFloat(m.P95TripS),
		strconv.Itoa(m.RouteChanges),
		strconv.Itoa(m.UnroutableVehicles),
		formatFloat(m.SaturatedEdgesPct),
		formatFloat(m.UtilizationStdDev),
		formatFloat(m.Fairness.MeanTimeLostS),
		strconv.Itoa(m.Fairness.FleetTrips),
		formatFloat(m.Fairness.MeanFleetTripS),
		strconv.Itoa(m.Fairness.BackgroundTrips),
		formatFloat(m.Fairness.MeanBackgroundTripS),
	}
	_, err := io.WriteString(w, strings.Join(fields[:], ",")+"\n")
	return err
}

// formatFloat renders a metric with a fixed number of decimals, so the same
// numbers always produce the same bytes regardless of how they were computed.
// It also folds negative zero, which 'f' would print as "-0.0000", into "0.0000".
func formatFloat(v float64) string {
	const places = 4
	if v == 0 {
		v = 0
	}
	return strconv.FormatFloat(v, 'f', places, 64)
}

// tieTolerance is how close two values must be to be called the same: one part
// per million. Identical traffic does not produce bit-identical sums, because the
// arms add the same quantities up through different code paths, and an
// interpolated percentile can shift further than that on its own. A relative
// rule covers all of them; the absolute floor keeps a value near zero comparable,
// since the metrics here are seconds. Real effects in these metrics are seconds,
// so nothing is hidden by it.
const tieTolerance = 1e-6

// ties reports whether two values are too close to tell apart.
func ties(base, fleet float64) bool {
	scale := math.Max(math.Abs(base), math.Abs(fleet))
	if scale < 1 {
		scale = 1
	}
	return math.Abs(fleet-base) <= tieTolerance*scale
}

// MetricSummary is the difference between the arms for one metric, averaged
// over the seeds: the mean of (FleetFlow - baseline), and how often each arm
// came out lower. Lower is better for every metric reported here, so "better"
// means the FleetFlow value was smaller; losses and ties are counted just as
// plainly, because this feeds docs/results.md rather than a claim.
type MetricSummary struct {
	Name     string
	MeanDiff float64
	Better   int
	Worse    int
	Tied     int
	Seeds    int
}

// Summaries is one MetricSummary per reported metric, in a fixed order.
type Summaries []MetricSummary

// summaryMetrics names the metrics the summary reports and how to read them.
var summaryMetrics = []struct {
	name  string
	value func(sim.Metrics) float64
}{
	{"average trip time", func(m sim.Metrics) float64 { return m.AverageTripS }},
	{"p95 trip time", func(m sim.Metrics) float64 { return m.P95TripS }},
	{"time lost", func(m sim.Metrics) float64 { return m.Fairness.MeanTimeLostS }},
}

// summarize pairs the arms by seed and reports, for each metric, the mean
// per-seed difference (FleetFlow - baseline) and the win/loss/tie counts.
func summarize(rows []Row) Summaries {
	var seeds []int64
	seen := make(map[int64]bool)
	baseline := make(map[int64]sim.Metrics)
	fleetflow := make(map[int64]sim.Metrics)

	for _, r := range rows {
		if !seen[r.Seed] {
			seen[r.Seed] = true
			seeds = append(seeds, r.Seed)
		}
		switch r.Arm {
		case armBaseline:
			baseline[r.Seed] = r.M
		case armFleetFlow:
			fleetflow[r.Seed] = r.M
		}
	}

	out := make(Summaries, 0, len(summaryMetrics))
	for _, metric := range summaryMetrics {
		s := MetricSummary{Name: metric.name}
		for _, seed := range seeds {
			base, okBase := baseline[seed]
			fleet, okFleet := fleetflow[seed]
			if !okBase || !okFleet {
				continue
			}
			s.Seeds++

			baseValue := metric.value(base)
			fleetValue := metric.value(fleet)
			diff := fleetValue - baseValue
			s.MeanDiff += diff
			switch {
			case ties(baseValue, fleetValue):
				s.Tied++
			case diff < 0:
				s.Better++
			default:
				s.Worse++
			}
		}
		if s.Seeds > 0 {
			s.MeanDiff /= float64(s.Seeds)
			if s.Better == 0 && s.Worse == 0 {
				// Every seed was a tie, so there is no difference to report -- not
				// even the accumulated noise, which would print as a sign and read
				// as one arm being ahead.
				s.MeanDiff = 0
			}
		}
		out = append(out, s)
	}
	return out
}

// writeSummary prints the difference between the arms to stderr. It states
// losses and ties as plainly as wins, so the output is a record, not a claim.
func writeSummary(w io.Writer, sums Summaries) {
	if len(sums) == 0 {
		return
	}
	fmt.Fprintf(w, "FleetFlow minus baseline over %d seeds; negative means FleetFlow lower:\n", sums[0].Seeds)
	for _, s := range sums {
		fmt.Fprintf(w, "  %-17s mean %+.*fs  better on %d, worse on %d, tied on %d of %d\n",
			s.Name, 4, s.MeanDiff, s.Better, s.Worse, s.Tied, s.Seeds)
	}
}
