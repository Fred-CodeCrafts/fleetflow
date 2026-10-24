package osrm_test

import (
	"context"
	"testing"

	"fleetflow/internal/osrm"
)

func TestFallbackRouter(t *testing.T) {
	router := osrm.NewFallbackRouter(nil)
	from := osrm.Coordinate{Lat: -6.1752, Lon: 106.7084}
	to := osrm.Coordinate{Lat: -6.1726, Lon: 106.7122}

	route, err := router.Route(context.Background(), from, to)
	if err != nil {
		t.Fatalf("fallback router returned error: %v", err)
	}
	if len(route.Geometry) < 2 {
		t.Fatalf("expected at least 2 points in geometry, got %d", len(route.Geometry))
	}
	if route.DistanceM <= 0 {
		t.Fatalf("expected distance > 0, got %f", route.DistanceM)
	}
	if route.DurationS <= 0 {
		t.Fatalf("expected duration > 0, got %f", route.DurationS)
	}

	if err := router.Health(context.Background()); err != nil {
		t.Fatalf("fallback router health returned error: %v", err)
	}
}
