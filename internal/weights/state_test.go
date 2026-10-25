package weights

import (
	"math"
	"testing"
	"time"
)

func TestNewEdgeState_StartsAtFreeFlow(t *testing.T) {
	s := NewEdgeState()

	if got := s.Rho(); got != 0 {
		t.Errorf("expected Rho() = 0 for a fresh edge, got %f", got)
	}
}

func TestObserve_SmoothsTowardsReport(t *testing.T) {
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	s := NewEdgeState()

	// First report: half free-flow speed -> rho 0.5, gamma 0.3.
	s.Observe(7.5, 15, 0.3, now)
	want := 0.3 * 0.5
	if !approxEqual(s.Rho(), want) {
		t.Fatalf("after first report: Rho() = %f, want %f", s.Rho(), want)
	}

	// Second report: stopped -> rho 1. History keeps most of its weight.
	s.Observe(0, 15, 0.3, now.Add(time.Second))
	want = 0.7*want + 0.3*1
	if !approxEqual(s.Rho(), want) {
		t.Errorf("after second report: Rho() = %f, want %f", s.Rho(), want)
	}
}

func TestObserve_GammaOneTakesRawReport(t *testing.T) {
	s := NewEdgeState()
	now := time.Now()

	s.Observe(0, 15, 0.3, now) // rho 1, smoothed down
	s.Observe(15, 15, 1, now)  // rho 0 taken as-is

	if got := s.Rho(); got != 0 {
		t.Errorf("expected Rho() = 0 with gamma 1, got %f", got)
	}
}

func TestObserve_ClampsGamma(t *testing.T) {
	s := NewEdgeState()
	now := time.Now()

	s.Observe(15, 15, -1, now) // gamma below 0 must behave like 0
	if got := s.Rho(); got != 0 {
		t.Errorf("expected Rho() = 0, got %f", got)
	}

	s.Observe(0, 15, 5, now) // gamma above 1 must behave like 1
	if got := s.Rho(); got != 1 {
		t.Errorf("expected Rho() = 1, got %f", got)
	}
}

func TestObserve_KeepsRhoInRange(t *testing.T) {
	s := NewEdgeState()
	now := time.Now()

	// Alternate extremes many times; smoothing must never leave [0, 1].
	for i := 0; i < 100; i++ {
		s.Observe(0, 15, 0.9, now)
		s.Observe(30, 15, 0.9, now)
		if rho := s.Rho(); rho < 0 || rho > 1 {
			t.Fatalf("iteration %d: Rho() = %f out of [0, 1]", i, rho)
		}
	}
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
