package weights

import (
	"sync"
	"testing"
	"time"
)

// TestConcurrentReportsAndDecay mirrors the real workload: many goroutines
// reporting on shared edges while one goroutine runs a customization pass that
// decays every edge and reads its cost. Run with -race to check the per-edge
// locking.
func TestConcurrentReportsAndDecay(t *testing.T) {
	const (
		edgeCount = 16
		workers   = 8
		rounds    = 200
		freeFlow  = 20.0
	)
	params := DefaultParams() // read-only, safe to share between goroutines
	t0 := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

	states := make([]*EdgeState, edgeCount)
	for i := range states {
		states[i] = NewEdgeState()
	}

	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				s := states[(worker+i)%edgeCount]
				now := t0.Add(time.Duration(i) * time.Millisecond)
				s.Observe(float64(i%int(freeFlow+1)), freeFlow, params.Gamma, now)
				_ = s.Rho()
			}
		}(w)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			now := t0.Add(time.Duration(i) * time.Millisecond)
			for _, s := range states {
				s.Decay(now, params.Lambda, params.Hold)
				_ = Cost(freeFlow, params.Alpha, params.Beta, s.Rho())
			}
		}
	}()

	wg.Wait()

	for i, s := range states {
		if rho := s.Rho(); rho < 0 || rho > 1 {
			t.Errorf("edge %d: Rho() = %f out of [0, 1]", i, rho)
		}
	}
}
