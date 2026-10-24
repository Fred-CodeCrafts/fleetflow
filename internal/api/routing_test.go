package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"fleetflow/internal/osrm"
)

type routeFunc func(context.Context, osrm.Coordinate, osrm.Coordinate) (osrm.Route, error)

func (f routeFunc) Route(ctx context.Context, from, to osrm.Coordinate) (osrm.Route, error) {
	return f(ctx, from, to)
}

func TestRouteDelegatesToOSRM(t *testing.T) {
	router := routeFunc(func(_ context.Context, from, to osrm.Coordinate) (osrm.Route, error) {
		if from != (osrm.Coordinate{Lat: -6.2, Lon: 106.8}) ||
			to != (osrm.Coordinate{Lat: -6.3, Lon: 106.9}) {
			t.Fatalf("coordinates = %+v -> %+v", from, to)
		}
		return osrm.Route{
			Geometry:  [][]float64{{106.8, -6.2}, {106.9, -6.3}},
			DistanceM: 1234,
			DurationS: 321,
		}, nil
	})
	s := testServer(t, WithRouter(router))
	rec := postJSON(t, s, "/api/route", `{"from":{"lat":-6.2,"lon":106.8},"to":{"lat":-6.3,"lon":106.9}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	body := decodeBody(t, rec)
	if body["provider"] != "OSRM" || body["distance_m"] != float64(1234) || body["duration_s"] != float64(321) {
		t.Fatalf("response = %v", body)
	}
	geometry, ok := body["geometry"].([]any)
	if !ok || len(geometry) != 2 {
		t.Fatalf("geometry = %v", body["geometry"])
	}
}

func TestRouteRejectsInvalidCoordinates(t *testing.T) {
	s := testServer(t, WithRouter(routeFunc(func(context.Context, osrm.Coordinate, osrm.Coordinate) (osrm.Route, error) {
		t.Fatal("router called for invalid coordinates")
		return osrm.Route{}, nil
	})))
	rec := postJSON(t, s, "/api/route", `{"from":{"lat":-91,"lon":106.8},"to":{"lat":-6.3,"lon":106.9}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
}

func TestRouteReturnsNotFoundWhenOSRMHasNoRoute(t *testing.T) {
	s := testServer(t, WithRouter(routeFunc(func(context.Context, osrm.Coordinate, osrm.Coordinate) (osrm.Route, error) {
		return osrm.Route{}, osrm.ErrNoRoute
	})))
	rec := postJSON(t, s, "/api/route", `{"from":{"lat":-6.2,"lon":106.8},"to":{"lat":-6.3,"lon":106.9}}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
}

func TestRouteSurfacesRoutingServiceFailure(t *testing.T) {
	s := testServer(t, WithRouter(routeFunc(func(context.Context, osrm.Coordinate, osrm.Coordinate) (osrm.Route, error) {
		return osrm.Route{}, errors.New("network connection refused")
	})))
	rec := postJSON(t, s, "/api/route", `{"from":{"lat":-6.2,"lon":106.8},"to":{"lat":-6.3,"lon":106.9}}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if got := decodeBody(t, rec)["message"]; got != "OSRM could not calculate a route; check that the routing service and Jakarta dataset are available" {
		t.Fatalf("message = %v", got)
	}
}
