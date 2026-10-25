package engine

import (
	"math/bits"
	"sync/atomic"
	"time"
)

// latencyBucketCount fixes the histogram shape. Bucket i covers the
// power-of-two microsecond range [2^i, 2^(i+1)) microseconds, so each bucket is
// twice as wide as the one below it and the range from one microsecond to about
// 4.19 seconds (2^22µs) is covered by 23 buckets. The last bucket is an overflow
// bucket that holds every query slower than that.
//
// A fixed array is deliberate: recording a query never allocates, never grows
// and never has to resize, so it stays off the hot path.
//
// The bucket width is also the resolution. A reported percentile is the lower
// bound of the bucket that holds it, so it is accurate to within a factor of two
// and never overstates the observed duration. Percentiles are not interpolated
// inside a bucket, because the histogram keeps counts and has discarded the
// individual durations that interpolation would need.
const latencyBucketCount = 23

// QueryLatency is the measured distribution of route-query durations.
type QueryLatency struct {
	P50, P99 time.Duration
	Samples  int64
}

// queryLatencyHistogram records route-query durations as a fixed set of atomic
// bucket counters. Every field is an atomic integer, so the query path records a
// duration without taking a lock and without allocating, and the metrics
// endpoint can read the distribution while queries are still being served;
// neither side ever waits for the other.
type queryLatencyHistogram struct {
	buckets [latencyBucketCount]atomic.Uint64
	samples atomic.Int64
}

// record counts one query that took d. It is called on the query path, so it
// does nothing that can block and no work whose cost depends on the number of
// samples already recorded.
func (h *queryLatencyHistogram) record(d time.Duration) {
	h.buckets[latencyBucketIndex(d)].Add(1)
	h.samples.Add(1)
}

// snapshot reads the counters and derives the requested percentiles. It may run
// concurrently with record; the bucket counts it sums are each read once, so it
// sees a consistent picture of how many queries fell in every bucket up to the
// moment it read them.
func (h *queryLatencyHistogram) snapshot() QueryLatency {
	samples := h.samples.Load()
	out := QueryLatency{Samples: samples}
	if samples == 0 {
		return out
	}

	var counts [latencyBucketCount]uint64
	for i := range counts {
		counts[i] = h.buckets[i].Load()
	}
	out.P50 = histogramPercentile(&counts, samples, 50)
	out.P99 = histogramPercentile(&counts, samples, 99)
	return out
}

// latencyBucketIndex returns the bucket that holds d: bucket i for a duration of
// 2^i microseconds or more but less than 2^(i+1). Anything below a microsecond,
// including a clock that reports no measurable time, shares bucket 0.
func latencyBucketIndex(d time.Duration) int {
	if d < time.Microsecond {
		return 0
	}
	us := uint64(d / time.Microsecond)
	i := bits.Len64(us) - 1
	if i >= latencyBucketCount {
		return latencyBucketCount - 1
	}
	return i
}

// latencyBucketLowerBound returns the smallest duration that can land in bucket
// i, which is what a percentile drawn from that bucket reports.
func latencyBucketLowerBound(i int) time.Duration {
	if i < 1 {
		return 0
	}
	return time.Duration(1<<uint(i)) * time.Microsecond
}

// histogramPercentile returns the lower bound of the bucket holding the
// nearest-rank percentile: the duration at rank ceil(percent% * samples),
// counted from the fastest bucket upward. The rank is at least 1, so even a
// single recorded query reports the bucket it fell in.
func histogramPercentile(counts *[latencyBucketCount]uint64, samples, percent int64) time.Duration {
	rank := (samples*percent + 99) / 100
	if rank < 1 {
		rank = 1
	}

	var cumulative uint64
	for i := range counts {
		cumulative += counts[i]
		if int64(cumulative) >= rank {
			return latencyBucketLowerBound(i)
		}
	}
	// A self-consistent histogram cannot sum to fewer than samples, so this is
	// unreachable; it only gives the function a total result.
	return latencyBucketLowerBound(latencyBucketCount - 1)
}

// QueryLatency reports the distribution of route-query durations: p50 and p99
// over every query the engine has answered, and how many queries that is. It
// reads atomic counters only, so the metrics endpoint can call it while the
// engine serves traffic.
//
// P50 and P99 are the lower bounds of the buckets that hold those ranks, so they
// are accurate to within a factor of two, since each bucket is twice the width
// of the one below it. Samples is how many queries were recorded; before the
// first query it is zero and both percentiles are zero.
func (e *Engine) QueryLatency() QueryLatency { return e.latency.snapshot() }
