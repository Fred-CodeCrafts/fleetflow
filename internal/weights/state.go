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
