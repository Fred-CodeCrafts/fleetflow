package osrm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrNoRoute = errors.New("no route found")

const maxResponseBytes = 4 << 20

type Coordinate struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type Route struct {
	Geometry  [][]float64 `json:"geometry"`
	DistanceM float64     `json:"distance_m"`
	DurationS float64     `json:"duration_s"`
}

type Router interface {
	Route(context.Context, Coordinate, Coordinate) (Route, error)
}

type HealthChecker interface {
	Health(context.Context) error
}

type Client struct {
	baseURL string
	client  *http.Client
}

func New(baseURL string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse OSRM URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("OSRM URL must be an http(s) URL without credentials, query, or fragment")
	}
	return &Client{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		client:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (c *Client) Route(ctx context.Context, from, to Coordinate) (Route, error) {
	if !validCoordinate(from) || !validCoordinate(to) {
		return Route{}, errors.New("coordinates are outside valid latitude/longitude ranges")
	}

	path := fmt.Sprintf("/route/v1/driving/%s,%s;%s,%s",
		format(from.Lon), format(from.Lat), format(to.Lon), format(to.Lat))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?overview=full&geometries=geojson&steps=false", nil)
	if err != nil {
		return Route{}, fmt.Errorf("create OSRM request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return Route{}, fmt.Errorf("request OSRM route: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Route{}, fmt.Errorf("OSRM returned HTTP %d", resp.StatusCode)
	}

	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Routes  []struct {
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
			Geometry struct {
				Coordinates [][]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&body); err != nil {
		return Route{}, fmt.Errorf("decode OSRM response: %w", err)
	}
	if body.Code == "NoRoute" || body.Code == "NoSegment" {
		return Route{}, ErrNoRoute
	}
	if body.Code != "Ok" || len(body.Routes) == 0 {
		if body.Message == "" {
			body.Message = "OSRM returned no route"
		}
		return Route{}, fmt.Errorf("OSRM route failed: %s", body.Message)
	}

	route := body.Routes[0]
	if len(route.Geometry.Coordinates) < 2 || route.Distance < 0 || route.Duration < 0 {
		return Route{}, errors.New("OSRM returned an invalid route")
	}
	for _, point := range route.Geometry.Coordinates {
		if len(point) != 2 || !validCoordinate(Coordinate{Lon: point[0], Lat: point[1]}) {
			return Route{}, errors.New("OSRM returned invalid route geometry")
		}
	}

	return Route{
		Geometry:  route.Geometry.Coordinates,
		DistanceM: route.Distance,
		DurationS: route.Duration,
	}, nil
}

func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/nearest/v1/driving/106.8456,-6.2088?number=1", nil)
	if err != nil {
		return fmt.Errorf("create OSRM health request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("check OSRM availability: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OSRM health check returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return fmt.Errorf("decode OSRM health response: %w", err)
	}
	if body.Code != "Ok" {
		return fmt.Errorf("OSRM health check returned code %q", body.Code)
	}
	return nil
}

func validCoordinate(point Coordinate) bool {
	return point.Lat >= -90 && point.Lat <= 90 && point.Lon >= -180 && point.Lon <= 180
}

func format(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
