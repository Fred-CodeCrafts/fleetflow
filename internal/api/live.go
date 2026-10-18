package api

import (
	"sort"
	"sync"
	"time"
)

// Live-vehicle tracking. The reports endpoint folds a speed into the weights of
// an edge and forgets who sent it, which is all routing needs; the dashboard
// needs one thing more, to show real traffic next to simulated traffic and label
// which is which (T43, PRD US-7). This keeps just enough to do that: the edge
// each vehicle was last heard from, and when.
//
// It is a last-seen register, not a position feed. A report names an edge, not a
// point, so a live vehicle is drawn on the edge it reported on and marked live;
// GPS traces would be a different (roadmap) feature, and pretending to have them
// would be worse than not having them.
const (
	// liveVehicleTTL is how long a vehicle stays on the live view after its last
	// report. It is deliberately longer than a reporting interval and short
	// enough that a vehicle which has left the network stops being drawn.
	liveVehicleTTL = 60 * time.Second
	// maxLiveVehicles bounds the register. A flooding client cannot make the
	// server hold an unbounded map: past this, the oldest entries are dropped.
	maxLiveVehicles = 512
)

// liveVehicle is one real vehicle as last reported.
type liveVehicle struct {
	ID       string
	EdgeID   string
	SpeedMPS float64
	// At is when the server accepted the report. Age is measured from here, so a
	// client that cannot keep its clock in step with the server's does not make
	// its own vehicles look stale.
	At time.Time
	// ObservedAt is the moment the report described, which is the observation the
	// smoothing used. It is earlier than At for a report that was queued or
	// replayed.
	ObservedAt time.Time
}

// liveVehicles is the register itself. It is written from the report handler and
// read from the stream, so every method takes the lock; the work under it is a
// map operation, which is why the stream can afford to read it on every frame.
type liveVehicles struct {
	mu       sync.Mutex
	seen     map[string]liveVehicle
	ttl      time.Duration
	capacity int
}

// newLiveVehicles returns a register holding at most capacity vehicles, each for
// at most ttl after its last report.
func newLiveVehicles(ttl time.Duration, capacity int) *liveVehicles {
	return &liveVehicles{
		seen:     make(map[string]liveVehicle),
		ttl:      ttl,
		capacity: capacity,
	}
}

// observe records that a vehicle reported on an edge just now. A report that
// describes a moment older than the ttl is not live traffic: it belongs in the
// weights (the caller has already folded it in) but a vehicle that was there a
// minute ago is not on the road now, and drawing it as live would be a lie.
func (l *liveVehicles) observe(id, edgeID string, speedMPS float64, observedAt time.Time) {
	now := time.Now()
	if now.Sub(observedAt) > l.ttl {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.dropStaleLocked(now)
	if _, known := l.seen[id]; !known && len(l.seen) >= l.capacity {
		// A new vehicle when the register is full: make room by forgetting the
		// quietest, rather than refusing the one that is reporting now.
		l.dropOldestLocked()
	}
	l.seen[id] = liveVehicle{
		ID:         id,
		EdgeID:     edgeID,
		SpeedMPS:   speedMPS,
		At:         now,
		ObservedAt: observedAt,
	}
}

// snapshot returns the vehicles still live at now, ordered by ID so that two
// frames built from the same state are identical.
func (l *liveVehicles) snapshot(now time.Time) []liveVehicle {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.dropStaleLocked(now)

	out := make([]liveVehicle, 0, len(l.seen))
	for _, v := range l.seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// dropStaleLocked forgets every vehicle that has not reported within the ttl.
func (l *liveVehicles) dropStaleLocked(now time.Time) {
	for id, v := range l.seen {
		if now.Sub(v.At) > l.ttl {
			delete(l.seen, id)
		}
	}
}

// dropOldestLocked forgets the vehicle that has been quiet longest, breaking
// ties by ID so that the choice does not depend on map iteration order.
func (l *liveVehicles) dropOldestLocked() {
	var oldestID string
	var oldestAt time.Time
	for id, v := range l.seen {
		if oldestID == "" || v.At.Before(oldestAt) || (v.At.Equal(oldestAt) && id < oldestID) {
			oldestID, oldestAt = id, v.At
		}
	}
	delete(l.seen, oldestID)
}
