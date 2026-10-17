package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// fixedClock returns a limiter whose clock the test controls.
func fixedClock(perSecond, burst float64) (*rateLimiter, *time.Time) {
	base := time.Unix(0, 0)
	l := newRateLimiter(perSecond, burst)
	l.now = func() time.Time { return base }
	return l, &base
}

func TestRateLimiter_AllowsTheBurstThenRefuses(t *testing.T) {
	l, _ := fixedClock(10, 5)

	for i := 1; i <= 5; i++ {
		if ok, _ := l.allow("203.0.113.9"); !ok {
			t.Fatalf("request %d was refused inside the burst", i)
		}
	}

	ok, retryAfter := l.allow("203.0.113.9")
	if ok {
		t.Fatal("a request past the burst was allowed")
	}
	if retryAfter <= 0 {
		t.Errorf("retryAfter = %v, want a positive wait", retryAfter)
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	l, base := fixedClock(10, 1) // one token per 100ms

	if ok, _ := l.allow("c"); !ok {
		t.Fatal("the first request was refused")
	}
	if ok, _ := l.allow("c"); ok {
		t.Fatal("a second request was allowed with an empty bucket")
	}

	*base = base.Add(100 * time.Millisecond)
	if ok, _ := l.allow("c"); !ok {
		t.Error("a request was refused even though the bucket had refilled")
	}
}

func TestRateLimiter_RefillIsCappedAtTheBurst(t *testing.T) {
	l, base := fixedClock(10, 2)

	// Spend the burst, then wait far longer than it takes to refill: a client
	// that has been quiet must not bank an unbounded allowance.
	l.allow("c")
	l.allow("c")

	*base = base.Add(time.Hour)

	allowed := 0
	for i := 0; i < 10; i++ {
		if ok, _ := l.allow("c"); ok {
			allowed++
		}
	}
	if allowed != 2 {
		t.Errorf("allowed %d requests after a long idle period, want the burst of 2", allowed)
	}
}

func TestRateLimiter_KeepsClientsApart(t *testing.T) {
	l, _ := fixedClock(1, 1)

	if ok, _ := l.allow("a"); !ok {
		t.Fatal("client a was refused its first request")
	}
	if ok, _ := l.allow("b"); !ok {
		t.Error("client b was refused, but it has a bucket of its own")
	}
}

func TestRateLimiter_DoesNotGrowWithoutBound(t *testing.T) {
	l, base := fixedClock(1, 1)

	// A flood of distinct addresses, each seen only once.
	for i := 0; i < maxRateBuckets+500; i++ {
		*base = base.Add(time.Millisecond)
		l.allow("client-" + strconv.Itoa(i))
	}

	l.mu.Lock()
	tracked := len(l.buckets)
	l.mu.Unlock()

	if tracked > maxRateBuckets {
		t.Errorf("tracked %d buckets, want at most %d", tracked, maxRateBuckets)
	}
}

func TestRateLimitMiddleware_RefusesPastTheBurst(t *testing.T) {
	l, _ := fixedClock(1, 1)

	calls := 0
	h := l.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	serve := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/reports", nil)
		req.RemoteAddr = "203.0.113.9:5555"
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := serve(); rec.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d, want 204", rec.Code)
	}

	rec := serve()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", rec.Code)
	}
	if calls != 1 {
		t.Errorf("handler ran %d times, want 1: a refused request must do nothing", calls)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("a refused response carries no Retry-After header")
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Code != "rate_limited" {
		t.Errorf("code = %q, want rate_limited", body.Code)
	}
}

func TestRateLimitMiddleware_AttributesByForwardedClient(t *testing.T) {
	l, _ := fixedClock(1, 1)

	h := l.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	send := func(forwarded string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/reports", nil)
		// Two different clients behind the same proxy socket.
		req.RemoteAddr = "10.0.0.1:5555"
		req.Header.Set("X-Forwarded-For", forwarded)
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := send("198.51.100.1"); code != http.StatusNoContent {
		t.Fatalf("first client status = %d, want 204", code)
	}
	// The same proxy address, a different client: it must get its own bucket.
	if code := send("198.51.100.2"); code != http.StatusNoContent {
		t.Errorf("second client status = %d, want 204: buckets must key on the client, not the proxy", code)
	}
	if code := send("198.51.100.1"); code != http.StatusTooManyRequests {
		t.Errorf("returning client status = %d, want 429", code)
	}
}
