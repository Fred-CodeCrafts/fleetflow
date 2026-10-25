package weights

import "time"

// Decay pulls the smoothed congestion back towards free-flow when an edge has
// gone quiet, and clears the report flag for the next customization cycle:
//
//	if !reported && now.Sub(lastReport) >= hold {
//	    rhoBar = rhoBar * (1 - lambda)
//	}
//
// The hold time matters: without it a road would snap back to free-flow as soon
// as the fleet avoided it, and vehicles would flood straight back in. lambda is
// the decay rate per cycle, clamped to [0, 1]. now is the current cycle time.
func (s *EdgeState) Decay(now time.Time, lambda float64, hold time.Duration) {
	l := clamp01(lambda)

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.reported && now.Sub(s.lastReport) >= hold {
		s.rhoBar *= 1 - l
	}
	s.reported = false
}
