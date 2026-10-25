package engine

import (
	"sync"
	"testing"
	"time"
)

// TestQueryLatency_ZeroWhenNothingRecorded covers the empty engine: before any
// query is made there are no samples and both percentiles are zero, so the
// metrics endpoint has nothing to report rather than a misleading minimum.
func TestQueryLatency_ZeroWhenNothingRecorded(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	got := e.QueryLatency()
	if got.Samples != 0 || got.P50 != 0 || got.P99 != 0 {
		t.Errorf("QueryLatency() on a fresh engine = %+v, want the zero value", got)
	}
}

// TestQueryLatency_PercentilesFromKnownDurations records a known multiset of
// durations directly into the histogram and checks the ranks the percentiles
// land on. The durations are exact powers of two microseconds so their buckets'
// lower bounds are the durations themselves, which makes the expected result
// exact rather than approximate.
func TestQueryLatency_PercentilesFromKnownDurations(t *testing.T) {
	const (
		fast = 1 << 10 * time.Microsecond // bucket 10, lower bound 1024µs
		slow = 1 << 15 * time.Microsecond // bucket 15, lower bound 32768µs
		rare = 1 << 20 * time.Microsecond // bucket 20, lower bound 1048576µs
	)

	var h queryLatencyHistogram
	for i := 0; i < 60; i++ {
		h.record(fast)
	}
	for i := 0; i < 39; i++ {
		h.record(slow)
	}
	h.record(rare)

	got := h.snapshot()
	if got.Samples != 100 {
		t.Fatalf("Samples = %d, want 100", got.Samples)
	}
	// Rank 50 of 100 falls in the 60 fast samples.
	if want := time.Duration(fast); got.P50 != want {
		t.Errorf("P50 = %s, want %s", got.P50, want)
	}
	// Rank 99 of 100 is the last of the 39 slow samples.
	if want := time.Duration(slow); got.P99 != want {
		t.Errorf("P99 = %s, want %s", got.P99, want)
	}
}

// TestQueryLatency_RecordsABelowMicrosecondQuery checks the fastest bucket: a
// duration that measures as less than a microsecond is still one sample, and
// reports the bucket lower bound of zero rather than being dropped.
func TestQueryLatency_RecordsABelowMicrosecondQuery(t *testing.T) {
	var h queryLatencyHistogram
	h.record(0)

	got := h.snapshot()
	if got.Samples != 1 {
		t.Errorf("Samples = %d, want 1", got.Samples)
	}
	if got.P50 != 0 || got.P99 != 0 {
		t.Errorf("percentiles = (%s, %s), want (0, 0)", got.P50, got.P99)
	}
}

// TestQueryLatency_CountsEachEntryPointOnce proves the delegation rule: Route,
// RouteWithVersion and RouteWithHeld are three separate entry points, but a
// query that enters at the top and funnels through RouteWithHeld is counted
// once, not once per hop.
func TestQueryLatency_CountsEachEntryPointOnce(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	if _, err := e.Route("A", "D"); err != nil {
		t.Fatalf("unexpected Route error: %v", err)
	}
	if _, _, err := e.RouteWithVersion("A", "D"); err != nil {
		t.Fatalf("unexpected RouteWithVersion error: %v", err)
	}
	if _, err := e.RouteWithHeld("A", "D", nil); err != nil {
		t.Fatalf("unexpected RouteWithHeld error: %v", err)
	}

	got := e.QueryLatency()
	if got.Samples != 3 {
		t.Errorf("Samples after three queries = %d, want 3", got.Samples)
	}
	if got.P50 > got.P99 {
		t.Errorf("P50 = %s exceeds P99 = %s", got.P50, got.P99)
	}
}

// TestQueryLatency_MonotonicUnderConcurrentQueries runs many queries from many
// goroutines and checks that every one is recorded and the percentiles stay
// ordered. Run it with -race: the point is that recording is atomic and the
// snapshot can be read while queries are in flight.
func TestQueryLatency_MonotonicUnderConcurrentQueries(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	const workers, perWorker = 8, 250

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine compares the count against its own last read, so
			// the monotonic check itself shares no memory between goroutines.
			var lastRead int64
			for i := 0; i < perWorker; i++ {
				if _, err := e.Route("A", "D"); err != nil {
					t.Errorf("unexpected route error: %v", err)
					return
				}
				if n := e.QueryLatency().Samples; n < lastRead {
					t.Errorf("Samples went backwards: %d after %d", n, lastRead)
					return
				} else {
					lastRead = n
				}
			}
		}()
	}
	wg.Wait()

	got := e.QueryLatency()
	if want := int64(workers * perWorker); got.Samples != want {
		t.Errorf("Samples = %d, want %d", got.Samples, want)
	}
	if got.P50 > got.P99 {
		t.Errorf("P50 = %s exceeds P99 = %s", got.P50, got.P99)
	}
}

// TestQueryLatency_RecordDoesNotAllocate guards the hot-path promise: recording a
// duration must add to fixed counters and return, with nothing for the garbage
// collector to reclaim on every query.
func TestQueryLatency_RecordDoesNotAllocate(t *testing.T) {
	var h queryLatencyHistogram
	if allocs := testing.AllocsPerRun(1000, func() {
		h.record(1500 * time.Microsecond)
	}); allocs != 0 {
		t.Errorf("record allocated %v times per call, want 0", allocs)
	}
}
