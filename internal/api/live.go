package api

import (
	"sort"
	"sync"
	"time"
)

const (
	liveVehicleTTL  = 60 * time.Second
	maxLiveVehicles = 512
)

type liveVehicle struct {
	ID         string
	Lat        float64
	Lon        float64
	SpeedMPS   float64
	At         time.Time
	ObservedAt time.Time
}

type liveVehicles struct {
	mu       sync.Mutex
	seen     map[string]liveVehicle
	ttl      time.Duration
	capacity int
}

func newLiveVehicles(ttl time.Duration, capacity int) *liveVehicles {
	return &liveVehicles{
		seen:     make(map[string]liveVehicle),
		ttl:      ttl,
		capacity: capacity,
	}
}

func (l *liveVehicles) observePosition(id string, lat, lon, speedMPS float64, observedAt time.Time) {
	now := time.Now()
	if now.Sub(observedAt) > l.ttl {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.dropStaleLocked(now)
	if _, known := l.seen[id]; !known && len(l.seen) >= l.capacity {
		l.dropOldestLocked()
	}
	l.seen[id] = liveVehicle{
		ID:         id,
		Lat:        lat,
		Lon:        lon,
		SpeedMPS:   speedMPS,
		At:         now,
		ObservedAt: observedAt,
	}
}

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

func (l *liveVehicles) dropStaleLocked(now time.Time) {
	for id, v := range l.seen {
		if now.Sub(v.At) > l.ttl {
			delete(l.seen, id)
		}
	}
}

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
