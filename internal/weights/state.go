// Package weights maintains the dynamic per-edge congestion state that the
// routing engine turns into edge costs. Route queries never touch this package
// directly: they read an immutable metric snapshot instead, so the locks here
// can never block a query.
package weights

import (
	"sync"
	"time"
)

// EdgeState is the mutable congestion state of a single road edge.
//
// Each edge owns its lock, so a report for one edge never blocks a report or a
// customization pass on another edge.
type EdgeState struct {
	mu sync.RWMutex

	// rhoBar is the smoothed congestion in [0, 1]; 0 is free-flow.
	rhoBar float64
	// lastReport is the time of the most recent accepted report.
	lastReport time.Time
	// reported is true when a report has arrived since the last decay pass.
	reported bool
}

// NewEdgeState returns an EdgeState at free-flow with no reports yet.
func NewEdgeState() *EdgeState {
	return &EdgeState{}
}

// Rho returns the current smoothed congestion in [0, 1].
func (s *EdgeState) Rho() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rhoBar
}

// Reset returns the edge to free flow, discarding its history. It exists for a
// simulation run that owns the weights and must start from an empty network;
// nothing else may call it, because a live edge's history is its only record of
// congestion.
func (s *EdgeState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rhoBar = 0
	s.lastReport = time.Time{}
	s.reported = false
}

// Observe folds one vehicle report into the smoothed congestion:
//
//	rhoBar = (1 - gamma) * rhoBar + gamma * rho
//
// gamma is the smoothing rate in [0, 1]: 1 takes the raw report, smaller values
// give the existing history more weight. It is clamped so an out-of-range
// parameter cannot push rhoBar outside its bounds. now is stored as the report
// time for the decay pass to use.
func (s *EdgeState) Observe(speedMPS, freeFlowMPS, gamma float64, now time.Time) {
	rho := Congestion(speedMPS, freeFlowMPS)
	g := clamp01(gamma)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.rhoBar = clamp01((1-g)*s.rhoBar + g*rho)
	s.lastReport = now
	s.reported = true
}
