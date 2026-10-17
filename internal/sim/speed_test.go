package sim

import (
	"math"
	"testing"
)

func TestEdgeSpeed(t *testing.T) {
	const (
		vFree    = 15.0
		capacity = 40.0
	)

	tests := []struct {
		name     string
		vFree    float64
		capacity float64
		n        int
		want     float64
	}{
		{name: "empty edge runs at free flow", vFree: vFree, capacity: capacity, n: 0, want: vFree},
		{name: "one vehicle is barely slower", vFree: vFree, capacity: capacity, n: 1,
			want: vFree / (1 + 0.15*math.Pow(1.0/40.0, 4))},
		{name: "at capacity", vFree: vFree, capacity: capacity, n: 40, want: vFree / 1.15},
		{name: "twice capacity", vFree: vFree, capacity: capacity, n: 80, want: vFree / (1 + 0.15*16)},
		{name: "zero capacity falls back to free flow", vFree: vFree, capacity: 0, n: 10, want: vFree},
		{name: "zero free-flow speed", vFree: 0, capacity: capacity, n: 5, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EdgeSpeed(tt.vFree, tt.capacity, tt.n)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("EdgeSpeed(%f, %f, %d) = %f, want %f", tt.vFree, tt.capacity, tt.n, got, tt.want)
			}
		})
	}
}

func TestEdgeSpeed_SlowsMonotonicallyWithOccupancy(t *testing.T) {
	const (
		vFree    = 13.89
		capacity = 100.0
	)

	previous := EdgeSpeed(vFree, capacity, 1)
	for n := 2; n <= 1000; n += 7 {
		got := EdgeSpeed(vFree, capacity, n)
		if got > previous {
			t.Fatalf("speed rose from %f to %f at n=%d", previous, got, n)
		}
		if got <= 0 {
			t.Fatalf("speed must stay positive, got %f at n=%d", got, n)
		}
		previous = got
	}
}
