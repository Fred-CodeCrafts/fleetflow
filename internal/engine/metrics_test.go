package engine

import (
	"fmt"
	"testing"
	"time"

	"fleetflow/internal/graph"
	"fleetflow/internal/weights"
)

func TestNew_RecordsThePublishTime(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	last := e.LastCustomize()
	if last.IsZero() {
		t.Fatal("expected the initial publish time to be recorded")
	}
	if age := e.MetricAge(time.Now()); age < 0 || age > time.Minute {
		t.Errorf("MetricAge() = %s, want a small non-negative age", age)
	}
}

func TestMetricAge_GrowsWithTime(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	last := e.LastCustomize()

	if got := e.MetricAge(last); got != 0 {
		t.Errorf("MetricAge at the publish instant = %s, want 0", got)
	}
	if got, want := e.MetricAge(last.Add(750*time.Millisecond)), 750*time.Millisecond; got != want {
		t.Errorf("MetricAge = %s, want %s", got, want)
	}
	// A time before the publish must not report a negative age.
	if got := e.MetricAge(last.Add(-time.Hour)); got != 0 {
		t.Errorf("MetricAge before the publish = %s, want 0", got)
	}
}

func TestCustomize_RecordsItsDuration(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	if got := e.CustomizeDuration(); got != 0 {
		t.Errorf("initial CustomizeDuration = %s, want 0", got)
	}

	before := time.Now()
	e.Customize()
	elapsed := time.Since(before)

	// A pass over a four-edge fixture finishes below the clock resolution on
	// this machine, so only require the recorded value to be sane here. The
	// synthetic-graph test below covers the positive case.
	if got := e.CustomizeDuration(); got < 0 || got > elapsed {
		t.Errorf("CustomizeDuration = %s, want between 0 and %s", got, elapsed)
	}
	if got := e.LastCustomize(); got.Before(before) {
		t.Errorf("LastCustomize = %s, want at or after %s", got, before)
	}
}

func TestCustomize_RecordsAMeasurableDurationOnABigGraph(t *testing.T) {
	g := syntheticGraph(t, 100000)
	e, err := New(g, NewDijkstraRouter(g), weights.DefaultParams())
	if err != nil {
		t.Fatalf("unexpected error creating engine: %v", err)
	}

	before := time.Now()
	e.Customize()
	elapsed := time.Since(before)

	got := e.CustomizeDuration()
	if got <= 0 {
		t.Errorf("CustomizeDuration = %s, want a positive duration", got)
	}
	if got > elapsed {
		t.Errorf("CustomizeDuration = %s exceeds the elapsed %s", got, elapsed)
	}
}

// syntheticGraph builds a graph with the requested number of edges between two
// nodes, which is enough to make a customization pass take measurable time.
func syntheticGraph(t *testing.T, edges int) *graph.Graph {
	t.Helper()

	g := graph.New()
	for _, id := range []string{"a", "b"} {
		if err := g.AddNode(graph.Node{ID: id}); err != nil {
			t.Fatalf("unexpected error adding node %s: %v", id, err)
		}
	}
	for i := 0; i < edges; i++ {
		e := graph.Edge{
			ID: fmt.Sprintf("e%d", i), From: "a", To: "b",
			LengthM: 100, SpeedLimitMPS: 10, Lanes: 2,
		}
		if err := g.AddEdge(e); err != nil {
			t.Fatalf("unexpected error adding edge %d: %v", i, err)
		}
	}
	return g
}

func TestBaseline_CustomizeDoesNotRefreshTheAge(t *testing.T) {
	g := loadFixture(t)
	e := newTestBaseline(t, g)

	first := e.LastCustomize()
	e.Customize()

	if got := e.LastCustomize(); !got.Equal(first) {
		t.Errorf("baseline LastCustomize moved from %s to %s", first, got)
	}
}
