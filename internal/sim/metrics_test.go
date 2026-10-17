package sim

import (
	"math"
	"testing"
)

func TestPercentile_NearestRank(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		p      float64
		want   float64
	}{
		{name: "empty", values: nil, p: 95, want: 0},
		{name: "single value", values: []float64{7}, p: 95, want: 7},
		{name: "median of one to ten", values: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, p: 50, want: 5},
		{name: "p95 of one to ten", values: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, p: 95, want: 10},
		{name: "p95 of one to twenty", values: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, p: 95, want: 19},
		{name: "p100 is the maximum", values: []float64{1, 2, 3}, p: 100, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := percentile(tt.values, tt.p); got != tt.want {
				t.Errorf("percentile(%v, %f) = %f, want %f", tt.values, tt.p, got, tt.want)
			}
		})
	}
}

func TestCollect_TripStatistics(t *testing.T) {
	g := openGraph(t)
	s := newTestSim(t, g, 1)
	v := placeSolo(t, s, []int{0}) // a ten-second trip

	for i := 0; i < 12 && v.State() == VehicleRunning; i++ {
		s.Step()
	}

	m := s.Collect()

	if m.TotalVehicles != 1 || m.CompletedTrips != 1 {
		t.Fatalf("TotalVehicles = %d, CompletedTrips = %d, want 1 and 1", m.TotalVehicles, m.CompletedTrips)
	}
	if math.Abs(m.AverageTripS-10) > 1e-9 {
		t.Errorf("AverageTripS = %f, want 10", m.AverageTripS)
	}
	if math.Abs(m.P95TripS-10) > 1e-9 {
		t.Errorf("P95TripS = %f, want 10", m.P95TripS)
	}
}

func TestCollect_CountsRouteChangesAndUnroutableVehicles(t *testing.T) {
	g := openGraph(t)
	s := newTestSim(t, g, 1)
	v := placeSolo(t, s, []int{0})
	v.routeChanges = 3

	if got := s.Collect().RouteChanges; got != 3 {
		t.Errorf("RouteChanges = %d, want 3", got)
	}
	if got := s.Collect().RouteChangesPerVehicle; math.Abs(got-3) > 1e-9 {
		t.Errorf("RouteChangesPerVehicle = %f, want 3", got)
	}

	placeSolo(t, s, nil)
	v.state = VehicleUnroutable
	if got := s.Collect().UnroutableVehicles; got != 1 {
		t.Errorf("UnroutableVehicles = %d, want 1", got)
	}
}

func TestCollect_SaturationAndUtilization(t *testing.T) {
	g := openGraph(t)
	g.Edges[0].Capacity = 2 // edge 0 holds two vehicles at capacity
	s := newTestSim(t, g, 1)
	placeSolo(t, s, []int{0}) // one vehicle: half full

	m := s.Collect()
	if m.SaturatedEdges != 0 {
		t.Errorf("SaturatedEdges = %d, want 0 at half capacity", m.SaturatedEdges)
	}
	// Utilizations are 0.5 and 0, so the mean is 0.25 and the population
	// standard deviation is 0.25.
	if math.Abs(m.UtilizationStdDev-0.25) > 1e-9 {
		t.Errorf("UtilizationStdDev = %f, want 0.25", m.UtilizationStdDev)
	}

	// A second vehicle on the edge fills it to capacity.
	s.Occupancy().Enter(0)

	m = s.Collect()
	if m.SaturatedEdges != 1 {
		t.Errorf("SaturatedEdges = %d, want 1", m.SaturatedEdges)
	}
	if got, want := m.SaturatedEdgesPct, 50.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("SaturatedEdgesPct = %f, want %f", got, want)
	}
}

func TestCollect_QuietRunHasNoCongestion(t *testing.T) {
	g := openGraph(t)
	s := newTestSim(t, g, 1)

	m := s.Collect()
	if m.CompletedTrips != 0 || m.AverageTripS != 0 || m.P95TripS != 0 {
		t.Errorf("expected empty trip statistics, got %+v", m)
	}
	if m.SaturatedEdges != 0 || m.UtilizationStdDev != 0 {
		t.Errorf("expected no congestion, got %+v", m)
	}
}
