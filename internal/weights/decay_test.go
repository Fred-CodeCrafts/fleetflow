package weights

import (
	"testing"
	"time"
)

func TestDecay_DoesNotDecayWithinHold(t *testing.T) {
	t0 := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	s := NewEdgeState()
	s.Observe(0, 15, 1, t0) // fully congested

	s.Decay(t0.Add(5*time.Second), 0.1, 10*time.Second)

	if !approxEqual(s.Rho(), 1) {
		t.Errorf("expected no decay within hold, Rho() = %f", s.Rho())
	}
}

func TestDecay_DecaysAfterHoldOnEveryQuietCycle(t *testing.T) {
	t0 := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	s := NewEdgeState()
	s.Observe(0, 15, 1, t0) // fully congested

	s.Decay(t0.Add(15*time.Second), 0.1, 10*time.Second)
	want := 1 * 0.9
	if !approxEqual(s.Rho(), want) {
		t.Fatalf("after first decay: Rho() = %f, want %f", s.Rho(), want)
	}

	s.Decay(t0.Add(25*time.Second), 0.1, 10*time.Second)
	want *= 0.9
	if !approxEqual(s.Rho(), want) {
		t.Errorf("after second decay: Rho() = %f, want %f", s.Rho(), want)
	}
}

func TestDecay_FreshReportBlocksDecay(t *testing.T) {
	t0 := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	s := NewEdgeState()
	s.Observe(0, 15, 1, t0)
	s.Observe(0, 15, 1, t0.Add(12*time.Second)) // report in this cycle

	s.Decay(t0.Add(15*time.Second), 0.1, 10*time.Second)

	if !approxEqual(s.Rho(), 1) {
		t.Errorf("a reported edge must not decay, Rho() = %f", s.Rho())
	}
}

func TestDecay_TrendsToFreeFlowWhenQuiet(t *testing.T) {
	t0 := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	s := NewEdgeState()
	s.Observe(0, 15, 1, t0)

	for i := 1; i <= 200; i++ {
		s.Decay(t0.Add(time.Duration(i*20)*time.Second), 0.1, 10*time.Second)
	}

	if got := s.Rho(); got > 0.01 {
		t.Errorf("expected Rho() to trend towards 0, got %f", got)
	}
}

func TestDecay_ClampsLambda(t *testing.T) {
	t0 := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

	s := NewEdgeState()
	s.Observe(0, 15, 1, t0)
	s.Decay(t0.Add(20*time.Second), 2, 10*time.Second) // lambda above 1 clamps to 1
	if got := s.Rho(); got != 0 {
		t.Errorf("expected Rho() = 0 with lambda 1, got %f", got)
	}

	s2 := NewEdgeState()
	s2.Observe(0, 15, 1, t0)
	s2.Decay(t0.Add(20*time.Second), -1, 10*time.Second) // negative lambda clamps to 0
	if got := s2.Rho(); got != 1 {
		t.Errorf("expected Rho() = 1 with lambda 0, got %f", got)
	}
}
