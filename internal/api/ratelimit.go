package api

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	// ratePerSecond is how many requests one client may make per second on a
	// public endpoint, and rateBurst how many it may make at once. They are sized
	// to leave a dashboard and a reporting fleet comfortable while stopping a
	// runaway loop.
	ratePerSecond = 100
	rateBurst     = 200

	// maxRateBuckets bounds how many clients are tracked at once. Past it the
	// least recently seen buckets are forgotten, so a flood of distinct addresses
	// cannot grow the map without limit.
	maxRateBuckets = 4096
)

// refusedRateLimited refuses a client that has spent its allowance. The message
// stays vague: naming the exact limit would only help someone tune their abuse.
var refusedRateLimited = &refusedError{
	status:  http.StatusTooManyRequests,
	code:    "rate_limited",
	message: "too many requests",
}

// rateLimiter gives every client its own token bucket. It is deliberately
// process-local: the service runs with max-instances=1 (DESIGN 3.8), so this is
// the real limit for the deployment rather than one of several replicas each
// allowing the full rate.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket

	perSecond float64
	burst     float64

	// now is injectable so tests can advance the clock instead of sleeping.
	now func() time.Time
}

// tokenBucket is one client's remaining allowance.
type tokenBucket struct {
	tokens float64
	last   time.Time
}

// newRateLimiter returns a limiter allowing perSecond requests per client with
// burst allowed at once.
func newRateLimiter(perSecond, burst float64) *rateLimiter {
	return &rateLimiter{
		buckets:   make(map[string]*tokenBucket),
		perSecond: perSecond,
		burst:     burst,
		now:       time.Now,
	}
}

// allow reports whether client may make a request now. The second result is how
// long it should wait first, and is zero when the request is allowed.
func (l *rateLimiter) allow(client string) (bool, time.Duration) {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, known := l.buckets[client]
	if !known {
		// Only a new client can grow the map, so this is where it is bounded.
		if len(l.buckets) >= maxRateBuckets {
			l.evictLocked(now)
		}
		bucket = &tokenBucket{tokens: l.burst, last: now}
		l.buckets[client] = bucket
	}

	// Refill for the time that has passed, never above the burst.
	if elapsed := now.Sub(bucket.last); elapsed > 0 {
		bucket.tokens = math.Min(l.burst, bucket.tokens+elapsed.Seconds()*l.perSecond)
		bucket.last = now
	}

	if bucket.tokens < 1 {
		// Report how long the client must wait to earn one whole token.
		wait := (1 - bucket.tokens) / l.perSecond
		return false, time.Duration(wait * float64(time.Second))
	}

	bucket.tokens--
	return true, 0
}

// evictLocked forgets buckets until the map is comfortably under the cap, so the
// insert that follows fits. It runs only when a new client arrives at the cap.
func (l *rateLimiter) evictLocked(now time.Time) {
	// A bucket that has refilled completely holds nothing a fresh bucket would
	// not, so those are the cheapest to forget and cost no decision.
	refill := time.Duration(l.burst / l.perSecond * float64(time.Second))
	for client, bucket := range l.buckets {
		if now.Sub(bucket.last) >= refill {
			delete(l.buckets, client)
		}
	}
	if len(l.buckets) < maxRateBuckets {
		return
	}

	// Still at the cap, so every tracked client is active: forget the least
	// recently seen quarter. Dropping a quarter rather than one entry means a
	// flood pays for this sort occasionally instead of on every request.
	type client struct {
		key  string
		last time.Time
	}
	ordered := make([]client, 0, len(l.buckets))
	for key, bucket := range l.buckets {
		ordered = append(ordered, client{key, bucket.last})
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].last.Before(ordered[j].last) })

	for _, c := range ordered[:len(ordered)-maxRateBuckets*3/4] {
		delete(l.buckets, c.key)
	}
}

// middleware refuses a client that has spent its allowance, answering 429 with a
// Retry-After hint so a well-behaved client backs off instead of retrying at once.
func (l *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, retryAfter := l.allow(clientIP(r))
		if !allowed {
			if seconds := int(math.Ceil(retryAfter.Seconds())); seconds > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
			}
			writeError(w, r, refusedRateLimited)
			return
		}

		next.ServeHTTP(w, r)
	})
}
