package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"fleetflow/internal/weights"
)

// TestConcurrentQueriesReportsAndCustomization drives one dynamic engine the way
// the simulator will: a customization loop, several reporters, and several route
// queries at the same time. It also queries a baseline engine concurrently.
//
// Run it with -race; the point is that queries never block on writes and never
// observe a half-built metric.
func TestConcurrentQueriesReportsAndCustomization(t *testing.T) {
	g := loadFixture(t)

	params := weights.DefaultParams()
	params.Delta = time.Millisecond

	dynamic, err := New(g, NewDijkstraRouter(g), params)
	if err != nil {
		t.Fatalf("unexpected error creating dynamic engine: %v", err)
	}
	baseline, err := NewBaseline(g, NewDijkstraRouter(g))
	if err != nil {
		t.Fatalf("unexpected error creating baseline engine: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		dynamic.Run(ctx)
		close(runDone)
	}()

	var wg sync.WaitGroup

	// Reporters: reports and decay passes on shared edge state.
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				edgeID := g.Edges[(worker+i)%len(g.Edges)].ID
				if err := dynamic.Report(edgeID, float64(i%15), time.Now()); err != nil {
					t.Errorf("unexpected report error: %v", err)
				}
				dynamic.DecayStale(time.Now())
			}
		}(worker)
	}

	// Queriers: dynamic engine.
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				path, err := dynamic.Route("A", "D")
				if err != nil {
					t.Errorf("unexpected dynamic route error: %v", err)
					return
				}
				if len(path.Edges) == 0 {
					t.Error("dynamic route returned no edges")
					return
				}
			}
		}()
	}

	// Queriers: baseline engine, sharing no state with the dynamic one.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			if _, err := baseline.Route("A", "D"); err != nil {
				t.Errorf("unexpected baseline route error: %v", err)
				return
			}
		}
	}()

	wg.Wait()
	cancel()

	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("customization loop did not stop after the context was cancelled")
	}
}
