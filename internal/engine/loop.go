package engine

import (
	"context"
	"time"
)

// DecayStale applies decay to every edge that has gone quiet since its last
// report. It is meant to be called once per customization cycle, before the
// metric is rebuilt. Baseline mode has no weights, so it does nothing.
func (e *Engine) DecayStale(now time.Time) {
	if e.static {
		return
	}
	for i := range e.states {
		e.states[i].Decay(now, e.params.Lambda, e.params.Hold)
	}
}

// CustomizeEvery reports the interval at which Run refreshes the weights. A
// simulation that keeps its own clock uses it to decide when to refresh.
func (e *Engine) CustomizeEvery() time.Duration { return e.params.Delta }

// Run customizes the metric on every Delta interval until ctx is cancelled, then
// returns cleanly. It blocks, so callers start it in its own goroutine and stop
// it by cancelling the context.
//
// A simulator that keeps its own clock can skip Run and call DecayStale and
// Customize on its own schedule instead.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.params.Delta)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			e.DecayStale(now)
			e.Customize()
		}
	}
}
