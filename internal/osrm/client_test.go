package osrm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouteRequestsDrivingRouteAndReturnsGeometry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/route/v1/driving/106.8,-6.2;106.9,-6.3" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("geometries") != "geojson" || r.URL.Query().Get("overview") != "full" {
			t.Errorf("query = %v", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"Ok","routes":[{"distance":1234.5,"duration":321,"geometry":{"coordinates":[[106.8,-6.2],[106.9,-6.3]]}}]}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	route, err := client.Route(context.Background(),
		Coordinate{Lat: -6.2, Lon: 106.8},
		Coordinate{Lat: -6.3, Lon: 106.9})
	if err != nil {
		t.Fatal(err)
	}
	if route.DistanceM != 1234.5 || route.DurationS != 321 || len(route.Geometry) != 2 {
		t.Fatalf("route = %+v", route)
	}
}

func TestRouteReportsNoRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"NoRoute","routes":[]}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Route(context.Background(), Coordinate{Lat: -6.2, Lon: 106.8}, Coordinate{Lat: -6.3, Lon: 106.9})
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("error = %v, want ErrNoRoute", err)
	}
}

func TestHealthChecksOSRMNearestEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nearest/v1/driving/106.8456,-6.2088" || r.URL.Query().Get("number") != "1" {
			t.Errorf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"code":"Ok","waypoints":[]}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNewRejectsUnsafeOrInvalidURL(t *testing.T) {
	for _, value := range []string{"", "file:///tmp/osrm", "https://user:pass@example.com"} {
		if _, err := New(value); err == nil {
			t.Errorf("New(%q) succeeded, want error", value)
		}
	}
}
