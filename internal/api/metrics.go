package api

import (
	"net/http"
	"time"

	"fleetflow/internal/sim"
)

// metricsResponse is the body of GET /api/metrics (DESIGN 3.5). It carries the
// engine's own numbers always, and both arms of the comparison run while one is
// going, so one request is enough to render the dashboard's metric strip.
type metricsResponse struct {
	Engine engineMetrics `json:"engine"`
	// Run is null when no run is going. A run that has just started reports its
	// demand with no trips finished yet, which is a different answer from none.
	Run *runMetrics `json:"run"`
}

// engineMetrics is what the routing engine can say about itself. These are the
// figures that show the hot path is doing what the design promises: a snapshot
// published on a schedule, and queries reading it while it ages.
type engineMetrics struct {
	// MetricVersion counts customization passes since startup. A version that
	// never moves means reports are arriving but nothing is being published.
	MetricVersion uint64 `json:"metric_version"`
	// MetricAgeS is how long ago the active snapshot was published, in seconds.
	MetricAgeS float64 `json:"metric_age_s"`
	// CustomizeMS is how long the most recent customization pass took, in
	// milliseconds.
	CustomizeMS float64 `json:"customize_ms"`
	// QueryLatencyMS is the distribution of route-query durations, in
	// milliseconds.
	QueryLatencyMS queryLatencyMetrics `json:"query_latency_ms"`
}

// queryLatencyMetrics is the distribution of route-query durations. It is
// approximate by construction, and says so rather than implying more precision
// than it has: the engine keeps a power-of-two bucket histogram and reports the
// lower bound of the bucket a percentile falls in, so a figure is accurate to
// within a factor of two and never overstates the delay (T42a).
type queryLatencyMetrics struct {
	P50     float64 `json:"p50"`
	P99     float64 `json:"p99"`
	Samples int64   `json:"samples"`
}

// runMetrics describes a live run and both arms of it.
type runMetrics struct {
	Seed       int64   `json:"seed"`
	Vehicles   int     `json:"vehicles"`
	FleetShare float64 `json:"fleet_share"`
	StepS      float64 `json:"step_s"`
	WindowS    float64 `json:"window_s"`

	Baseline  armMetrics `json:"baseline"`
	FleetFlow armMetrics `json:"fleet_flow"`
}

// armMetrics is one arm of a run: the headline figures a viewer compares, plus
// the fairness check that must be read with them rather than instead of them.
type armMetrics struct {
	TotalVehicles          int     `json:"total_vehicles"`
	CompletedTrips         int     `json:"completed_trips"`
	AverageTripS           float64 `json:"average_trip_s"`
	P95TripS               float64 `json:"p95_trip_s"`
	RouteChanges           int     `json:"route_changes"`
	RouteChangesPerVehicle float64 `json:"route_changes_per_vehicle"`
	// UnroutableVehicles counts vehicles that never found a path. It should stay
	// zero; anything else means a route request failed.
	UnroutableVehicles int `json:"unroutable_vehicles"`
	// ReportsFailed counts reports this arm's fleet service refused. It should
	// stay zero: anything else means the arm has been routing on weights its own
	// traffic never reached, and a comparison read without it could call that
	// "no difference between the arms".
	ReportsFailed int `json:"reports_failed"`

	// An edge is saturated when it carries at least its capacity in vehicles.
	// The percentage is over the whole graph, and both figures are derived from
	// the same capacity the speed model uses, so the number is not a black box.
	SaturatedEdges    int     `json:"saturated_edges"`
	SaturatedEdgesPct float64 `json:"saturated_edges_pct"`
	UtilizationStdDev float64 `json:"utilization_std_dev"`

	Fairness fairnessMetrics `json:"fairness"`
}

// fairnessMetrics is the US-6 fairness check. It sits beside the headline figures
// and is not one of them: it says what the trips cost against free flow and how
// the two populations of vehicles were treated. None of it says that FleetFlow
// wins, and a result is worth reading only with it in view.
type fairnessMetrics struct {
	MeanTimeLostS  float64 `json:"mean_time_lost_s"`
	TotalTimeLostS float64 `json:"total_time_lost_s"`

	FleetTrips          int     `json:"fleet_trips"`
	MeanFleetTripS      float64 `json:"mean_fleet_trip_s"`
	BackgroundTrips     int     `json:"background_trips"`
	MeanBackgroundTripS float64 `json:"mean_background_trip_s"`
}

// metricsOf converts one arm's collected metrics into their wire shape. It is
// written out rather than serialized directly so the JSON stays stable when the
// simulator's own structs are reorganized.
func metricsOf(m sim.Metrics) armMetrics {
	return armMetrics{
		TotalVehicles:          m.TotalVehicles,
		CompletedTrips:         m.CompletedTrips,
		AverageTripS:           m.AverageTripS,
		P95TripS:               m.P95TripS,
		RouteChanges:           m.RouteChanges,
		RouteChangesPerVehicle: m.RouteChangesPerVehicle,
		UnroutableVehicles:     m.UnroutableVehicles,
		ReportsFailed:          m.ReportsFailed,
		SaturatedEdges:         m.SaturatedEdges,
		SaturatedEdgesPct:      m.SaturatedEdgesPct,
		UtilizationStdDev:      m.UtilizationStdDev,
		Fairness: fairnessMetrics{
			MeanTimeLostS:       m.Fairness.MeanTimeLostS,
			TotalTimeLostS:      m.Fairness.TotalTimeLostS,
			FleetTrips:          m.Fairness.FleetTrips,
			MeanFleetTripS:      m.Fairness.MeanFleetTripS,
			BackgroundTrips:     m.Fairness.BackgroundTrips,
			MeanBackgroundTripS: m.Fairness.MeanBackgroundTripS,
		},
	}
}

// handleMetrics reports the engine's numbers and, while a run is going, both arms
// of it. It reads snapshots only: the engine's active metric and the run's last
// published one, so the request never waits on a query or on a simulation step.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.eng == nil {
		writeError(w, r, errNoEngine)
		return
	}

	writeJSON(w, http.StatusOK, s.metricsResponse())
}

// metricsResponse builds the body of GET /api/metrics. The event stream sends the
// same object as its metrics event, so a client parses one shape whether it asks
// once or watches.
func (s *Server) metricsResponse() metricsResponse {
	now := time.Now()
	latency := s.eng.QueryLatency()

	body := metricsResponse{
		Engine: engineMetrics{
			MetricVersion: s.eng.ActiveMetric().Version(),
			MetricAgeS:    s.eng.MetricAge(now).Seconds(),
			CustomizeMS:   msOf(s.eng.CustomizeDuration()),
			QueryLatencyMS: queryLatencyMetrics{
				P50:     msOf(latency.P50),
				P99:     msOf(latency.P99),
				Samples: latency.Samples,
			},
		},
	}
	if live := s.currentRun(); live != nil {
		body.Run = live.metrics()
	}
	return body
}

// msOf converts a duration to milliseconds. Durations are recorded to the
// microsecond, which is the finest resolution either figure has, so nothing is
// lost by reporting them this way.
func msOf(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}
