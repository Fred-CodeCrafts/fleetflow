package main

import (
	"bytes"
	"flag"
	"io"
	"math"
	"strings"
	"testing"

	"fleetflow/internal/sim"
)

// baselineMetrics is a fully populated Metrics so every CSV column is exercised.
func baselineMetrics() sim.Metrics {
	return sim.Metrics{
		TotalVehicles:      10,
		CompletedTrips:     9,
		AverageTripS:       12.3456,
		P95TripS:           20.5,
		RouteChanges:       3,
		UnroutableVehicles: 1,
		SaturatedEdgesPct:  4.1667,
		UtilizationStdDev:  0.5,
		Fairness: sim.Fairness{
			MeanTimeLostS:       1.25,
			FleetTrips:          5,
			MeanFleetTripS:      10,
			BackgroundTrips:     4,
			MeanBackgroundTripS: 15,
		},
	}
}

func fleetFlowMetrics() sim.Metrics {
	return sim.Metrics{
		TotalVehicles:      10,
		CompletedTrips:     9,
		AverageTripS:       11,
		P95TripS:           19,
		RouteChanges:       0,
		UnroutableVehicles: 0,
		SaturatedEdgesPct:  2,
		UtilizationStdDev:  0.25,
		Fairness: sim.Fairness{
			MeanTimeLostS:       0.75,
			FleetTrips:          5,
			MeanFleetTripS:      9,
			BackgroundTrips:     4,
			MeanBackgroundTripS: 13,
		},
	}
}

func TestWriteCSV_HeaderAndRows(t *testing.T) {
	rows := []Row{
		{Seed: 7, Arm: armBaseline, M: baselineMetrics()},
		{Seed: 7, Arm: armFleetFlow, M: fleetFlowMetrics()},
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, rows); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	wantHeader := "seed,arm,vehicles,completed_trips,average_trip_s,p95_trip_s,route_changes," +
		"unroutable_vehicles,saturated_edges_pct,utilization_std_dev,mean_time_lost_s," +
		"fleet_trips,mean_fleet_trip_s,background_trips,mean_background_trip_s"
	if csvHeader != wantHeader {
		t.Errorf("header mismatch:\n got %q\nwant %q", csvHeader, wantHeader)
	}

	wantBaseline := "7,baseline,10,9,12.3456,20.5000,3,1,4.1667,0.5000,1.2500,5,10.0000,4,15.0000"
	wantFleetFlow := "7,fleetflow,10,9,11.0000,19.0000,0,0,2.0000,0.2500,0.7500,5,9.0000,4,13.0000"

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), buf.String())
	}
	if lines[0] != wantHeader {
		t.Errorf("line 0 = %q, want the header", lines[0])
	}
	if lines[1] != wantBaseline {
		t.Errorf("baseline row mismatch:\n got %q\nwant %q", lines[1], wantBaseline)
	}
	if lines[2] != wantFleetFlow {
		t.Errorf("fleetflow row mismatch:\n got %q\nwant %q", lines[2], wantFleetFlow)
	}

	for i, line := range lines {
		if got := len(strings.Split(line, ",")); got != 15 {
			t.Errorf("line %d has %d fields, want 15", i, got)
		}
	}
}

// TestWriteCSV_IsByteIdentical guards the "same flags, same CSV" promise: the
// same rows must serialise to the same bytes every time.
func TestWriteCSV_IsByteIdentical(t *testing.T) {
	rows := []Row{
		{Seed: 1, Arm: armBaseline, M: baselineMetrics()},
		{Seed: 1, Arm: armFleetFlow, M: fleetFlowMetrics()},
		{Seed: 2, Arm: armBaseline, M: baselineMetrics()},
		{Seed: 2, Arm: armFleetFlow, M: fleetFlowMetrics()},
	}

	var first, second bytes.Buffer
	if err := writeCSV(&first, rows); err != nil {
		t.Fatalf("first writeCSV: %v", err)
	}
	if err := writeCSV(&second, rows); err != nil {
		t.Fatalf("second writeCSV: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Errorf("two writes of the same rows differ:\n%s\n---\n%s", first.String(), second.String())
	}
}

func TestFormatFloat_FoldsNegativeZero(t *testing.T) {
	if got := formatFloat(math.Copysign(0, -1)); got != "0.0000" {
		t.Errorf("formatFloat(-0) = %q, want %q", got, "0.0000")
	}
	if got := formatFloat(20.5); got != "20.5000" {
		t.Errorf("formatFloat(20.5) = %q, want %q", got, "20.5000")
	}
}

func TestSummarize_ArithmeticAndLosses(t *testing.T) {
	avg := func(a, p, lost float64) sim.Metrics {
		return sim.Metrics{AverageTripS: a, P95TripS: p, Fairness: sim.Fairness{MeanTimeLostS: lost}}
	}

	// Seed 1: FleetFlow better on all three. Seed 2: worse on all three.
	// Seed 3: identical, so all three tie. The mean differences are positive,
	// i.e. FleetFlow loses overall — the summary must say so as plainly as a win.
	rows := []Row{
		{Seed: 1, Arm: armBaseline, M: avg(10, 20, 5)},
		{Seed: 1, Arm: armFleetFlow, M: avg(9, 18, 4)},
		{Seed: 2, Arm: armBaseline, M: avg(10, 20, 5)},
		{Seed: 2, Arm: armFleetFlow, M: avg(13, 26, 7)},
		{Seed: 3, Arm: armBaseline, M: avg(10, 20, 5)},
		{Seed: 3, Arm: armFleetFlow, M: avg(10, 20, 5)},
	}

	got := summarize(rows)
	if len(got) != 3 {
		t.Fatalf("got %d summaries, want 3", len(got))
	}

	wantNames := []string{"average trip time", "p95 trip time", "time lost"}
	wantDiffs := []float64{2.0 / 3.0, 4.0 / 3.0, 1.0 / 3.0}

	for i, s := range got {
		if s.Name != wantNames[i] {
			t.Errorf("summary %d name = %q, want %q", i, s.Name, wantNames[i])
		}
		if math.Abs(s.MeanDiff-wantDiffs[i]) > 1e-9 {
			t.Errorf("%s: MeanDiff = %v, want %v", s.Name, s.MeanDiff, wantDiffs[i])
		}
		if s.Better != 1 || s.Worse != 1 || s.Tied != 1 || s.Seeds != 3 {
			t.Errorf("%s: better=%d worse=%d tied=%d seeds=%d, want 1/1/1/3",
				s.Name, s.Better, s.Worse, s.Tied, s.Seeds)
		}
	}
}

// The arms add the same quantities up through different code paths, so identical
// traffic does not produce bit-identical sums, and an interpolated percentile can
// move further than that on its own. A difference that small is not a result:
// reporting one would be the worst kind of wrong, a win that is nothing but the
// order two numbers were added in.
func TestSummarize_CountsFloatNoiseAsATie(t *testing.T) {
	base := sim.Metrics{AverageTripS: 118.45752, P95TripS: 199.76782}
	fleet := sim.Metrics{AverageTripS: 118.45752000000001, P95TripS: 199.76782 + 2e-5}

	got := summarize([]Row{
		{Seed: 1, Arm: armBaseline, M: base},
		{Seed: 1, Arm: armFleetFlow, M: fleet},
	})

	for _, s := range got {
		if s.Better != 0 || s.Worse != 0 || s.Tied != 1 {
			t.Errorf("%s: better=%d worse=%d tied=%d, want every seed tied",
				s.Name, s.Better, s.Worse, s.Tied)
		}
		if s.MeanDiff != 0 {
			t.Errorf("%s: MeanDiff = %v, want no difference when every seed ties", s.Name, s.MeanDiff)
		}
	}
}

func TestWriteSummary_StatesLossesAndTies(t *testing.T) {
	rows := []Row{
		{Seed: 1, Arm: armBaseline, M: sim.Metrics{AverageTripS: 10}},
		{Seed: 1, Arm: armFleetFlow, M: sim.Metrics{AverageTripS: 12}},
	}

	var buf bytes.Buffer
	writeSummary(&buf, summarize(rows))
	out := buf.String()

	for _, want := range []string{"worse on 1", "better on 0", "tied on 0", "negative means FleetFlow lower"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary %q does not contain %q", out, want)
		}
	}
}

func TestParseFlags_Defaults(t *testing.T) {
	opts, err := parseFlags(nil, io.Discard)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if opts.graph != "data/graph.json" {
		t.Errorf("graph = %q, want data/graph.json", opts.graph)
	}
	if opts.seedFrom != 1 || opts.seedTo != 10 {
		t.Errorf("seed range = %d..%d, want 1..10", opts.seedFrom, opts.seedTo)
	}
	if opts.out != "" {
		t.Errorf("out = %q, want empty (stdout)", opts.out)
	}
	if opts.maxSteps != defaultMaxSteps {
		t.Errorf("max-steps = %d, want %d", opts.maxSteps, defaultMaxSteps)
	}
	if opts.cfg.Seed != 0 {
		t.Errorf("cfg.Seed = %d, want 0 (set per seed in collect)", opts.cfg.Seed)
	}
	if opts.cfg.Vehicles != sim.DefaultConfig().Vehicles {
		t.Errorf("vehicles = %d, want %d", opts.cfg.Vehicles, sim.DefaultConfig().Vehicles)
	}
	if opts.params.Alpha != 1.5 || opts.params.Hold.Seconds() != 10 {
		t.Errorf("params = %+v, want the DESIGN defaults", opts.params)
	}
}

func TestParseFlags_Overrides(t *testing.T) {
	opts, err := parseFlags([]string{
		"-graph", "g.json", "-seed-from", "3", "-seed-to", "5",
		"-vehicles", "42", "-fleet-share", "0.25", "-delta", "2.5", "-out", "x.csv",
	}, io.Discard)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if opts.graph != "g.json" || opts.seedFrom != 3 || opts.seedTo != 5 || opts.cfg.Vehicles != 42 ||
		opts.cfg.FleetShare != 0.25 || opts.out != "x.csv" {
		t.Errorf("options not applied: %+v", opts)
	}
	if opts.params.Delta.Seconds() != 2.5 {
		t.Errorf("delta = %v, want 2.5s", opts.params.Delta)
	}
}

func TestParseFlags_RejectsBadRanges(t *testing.T) {
	cases := map[string][]string{
		"seed range reversed":    {"-seed-from", "10", "-seed-to", "1"},
		"fleet share above one":  {"-fleet-share", "1.5"},
		"negative vehicles":      {"-vehicles", "-1"},
		"non-positive step":      {"-step-s", "0"},
		"threshold above one":    {"-reroute-threshold", "1"},
		"invalid gamma":          {"-gamma", "2"},
		"non-positive max steps": {"-max-steps", "0"},
	}
	for name, args := range cases {
		if _, err := parseFlags(args, io.Discard); err == nil {
			t.Errorf("%s: parseFlags accepted %v, want an error", name, args)
		}
	}
}

func TestParseFlags_HelpIsNotAnError(t *testing.T) {
	if _, err := parseFlags([]string{"-h"}, io.Discard); err != flag.ErrHelp {
		t.Errorf("parseFlags(-h) = %v, want flag.ErrHelp", err)
	}
}
