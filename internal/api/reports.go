package api

import (
	"math"
	"net/http"
	"time"

	"fleetflow/internal/domain"
)

// maxReportSpeedMPS bounds the speed a report may carry. A vehicle is not a
// rocket: anything past this is a unit mistake (km/h sent as m/s) or a corrupt
// sensor, and folding it in would drag the whole edge toward free flow. 90 m/s
// is 324 km/h, comfortably above any road vehicle and far above the 11-14 m/s
// limits the road data carries.
const maxReportSpeedMPS = 90.0

// Error codes for input the report endpoint refuses. They travel in API error
// bodies, so they are part of the public contract.
const (
	codeVehicleIDRequired = "vehicle_id_required"
	codeEdgeIDRequired    = "edge_id_required"
	codeSpeedOutOfRange   = "speed_out_of_range"
	codeTimestampInvalid  = "ts_invalid"
)

// reportRequest is the body of POST /api/reports (DESIGN 3.5).
type reportRequest struct {
	VehicleID string  `json:"vehicle_id"`
	EdgeID    string  `json:"edge_id"`
	SpeedMPS  float64 `json:"speed_mps"`
	// TS is when the observation was made, in Unix milliseconds. It is optional:
	// a client that omits it is reporting "now", which is what a live source
	// means. A client replaying history sends it so the smoothing reflects when
	// the observation happened rather than when it was uploaded.
	TS *int64 `json:"ts,omitempty"`
}

// handleReport folds one vehicle report into the congestion state of the edge it
// names, and answers 202: the report is recorded, and there is nothing to hand
// back. It writes to the weight store only and never touches PostgreSQL
// (DESIGN 3.5).
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	if s.eng == nil {
		writeError(w, r, errNoEngine)
		return
	}

	var req reportRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	observedAt, err := req.observeTime()
	if err != nil {
		writeError(w, r, err)
		return
	}

	if err := s.eng.Report(req.EdgeID, req.SpeedMPS, observedAt); err != nil {
		writeError(w, r, err)
		return
	}

	// A real vehicle that has just reported is on the road now. The weights have
	// already taken the speed; this is what lets the dashboard show live traffic
	// beside simulated traffic and label which is which (T43).
	s.live.observe(req.VehicleID, req.EdgeID, req.SpeedMPS, observedAt)

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// observeTime checks the report and returns the moment it describes. It is the
// one place a report is judged, so a client cannot be told two different things
// about the same request.
func (r reportRequest) observeTime() (time.Time, error) {
	if r.VehicleID == "" {
		return time.Time{}, domain.Invalid(codeVehicleIDRequired, "vehicle_id is required")
	}
	if r.EdgeID == "" {
		return time.Time{}, domain.Invalid(codeEdgeIDRequired, "edge_id is required")
	}
	if !validReportSpeed(r.SpeedMPS) {
		return time.Time{}, domain.Invalid(codeSpeedOutOfRange,
			"speed_mps must be greater than 0 and at most %g", maxReportSpeedMPS)
	}

	if r.TS == nil {
		return time.Now(), nil
	}
	if *r.TS <= 0 {
		return time.Time{}, domain.Invalid(codeTimestampInvalid, "ts must be a Unix time in milliseconds")
	}
	return time.UnixMilli(*r.TS), nil
}

// validReportSpeed reports whether a measured speed is one the smoothing should
// accept. NaN is checked explicitly because both comparisons against it are
// false, so it would otherwise pass every bound.
func validReportSpeed(speed float64) bool {
	return !math.IsNaN(speed) && speed > 0 && speed <= maxReportSpeedMPS
}
