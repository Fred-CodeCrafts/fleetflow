package api

import (
	"math"
	"net/http"
	"time"

	"fleetflow/internal/domain"
	"fleetflow/internal/engine"
)

// maxReportSpeedMPS bounds the speed a report may carry. A vehicle is not a
// rocket: anything past this is a unit mistake (km/h sent as m/s) or a corrupt
// sensor, and folding it in would drag the whole edge toward free flow. 90 m/s
// is 324 km/h, comfortably above any road vehicle and far above the 11-14 m/s
// limits the road data carries.
const maxReportSpeedMPS = 90.0

// maxReportSkew is how far ahead of this server's clock a report may be dated
// before it is refused. A source's clock running a little fast is ordinary, so
// this is an allowance rather than a requirement; an observation from next year
// is neither. It is bounded because a report's timestamp is stored on the edge
// and decay only resumes once the hold time has passed since it, so one report
// dated far ahead would hold that road at its last congestion until then.
const maxReportSkew = 5 * time.Minute

// Error codes for input the report endpoint refuses. They travel in API error
// bodies, so they are part of the public contract.
const (
	codeVehicleIDRequired = "vehicle_id_required"
	codeEdgeIDRequired    = "edge_id_required"
	codeSpeedOutOfRange   = "speed_out_of_range"
	codeTimestampInvalid  = "ts_invalid"
	codeTimestampInFuture = "ts_in_future"
	// codeBatchTooLarge refuses a batch bigger than maxBatchReports. It is its own
	// code because the batch endpoint has a bound the single endpoint does not.
	codeBatchTooLarge = "batch_too_large"
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
	_, eng := s.networkSnapshot()
	if eng == nil {
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

	if err := s.foldReport(r, eng, req, observedAt); err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// batchReportRequest is the body of POST /api/reports/batch: one simulated
// instant's observations, one per edge.
type batchReportRequest struct {
	Observations []reportRequest `json:"observations"`
}

// maxBatchReports bounds one batch request. A step reports at most one
// observation per edge, so this is a graph-sized bound rather than a vehicle
// count; it exists so a single request cannot ask the process to fold an
// unbounded number of reports into the weights.
const maxBatchReports = 200_000

// handleReportBatch folds a step's observations into the weights and answers 202
// with how many it accepted.
//
// It is the batch form of POST /api/reports, and it exists because congestion is a
// property of an edge: every vehicle on an edge observes the same speed, so a
// 60,000-vehicle step carries a few thousand distinct observations. Sent one at a
// time they were 30,000 serial round trips per step, which advanced a run two
// simulated seconds in forty seconds of wall clock.
//
// A batch is all-or-nothing on validation: one malformed observation rejects the
// request and none are folded, so a client never has to work out which half of a
// batch landed.
func (s *Server) handleReportBatch(w http.ResponseWriter, r *http.Request) {
	_, eng := s.networkSnapshot()
	if eng == nil {
		writeError(w, r, errNoEngine)
		return
	}

	var req batchReportRequest
	if err := decodeJSONLimit(w, r, &req, maxBatchBodyBytes); err != nil {
		writeError(w, r, err)
		return
	}
	if len(req.Observations) > maxBatchReports {
		writeError(w, r, domain.Invalid(codeBatchTooLarge,
			"a batch may carry at most %d observations", maxBatchReports))
		return
	}

	// Validate the whole batch first, so a bad entry cannot leave the weights
	// half updated.
	observedAt := make([]time.Time, len(req.Observations))
	for i, obs := range req.Observations {
		at, err := obs.observeTime()
		if err != nil {
			writeError(w, r, err)
			return
		}
		observedAt[i] = at
	}

	for i, obs := range req.Observations {
		if err := s.foldReport(r, eng, obs, observedAt[i]); err != nil {
			writeError(w, r, err)
			return
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":   "accepted",
		"accepted": len(req.Observations),
	})
}

// foldReport folds one validated report into the weights, and records it as live
// traffic unless it came from the simulator. It is the whole of what a report
// does, so the single and batch endpoints cannot drift apart.
func (s *Server) foldReport(r *http.Request, eng *engine.Engine, req reportRequest, observedAt time.Time) error {
	if err := eng.Report(req.EdgeID, req.SpeedMPS, observedAt); err != nil {
		return err
	}

	// A real vehicle that has just reported is on the road now. The weights have
	// already taken the speed; this is what lets the dashboard show live traffic
	// beside simulated traffic and label which is which (T43).
	//
	// The simulator's own reports are not real traffic, however they arrive. With
	// SIM_API_KEY set they come through this very endpoint, and registering them
	// would draw simulated vehicles as live and evict real ones from a bounded
	// register.
	if !s.isInternalKey(r) {
		s.live.observe(req.VehicleID, req.EdgeID, req.SpeedMPS, observedAt)
	}
	return nil
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

	observedAt := time.UnixMilli(*r.TS)
	if observedAt.After(time.Now().Add(maxReportSkew)) {
		return time.Time{}, domain.Invalid(codeTimestampInFuture,
			"ts must not be more than %s ahead of this server's clock", maxReportSkew)
	}
	return observedAt, nil
}

// validReportSpeed reports whether a measured speed is one the smoothing should
// accept. NaN is checked explicitly because both comparisons against it are
// false, so it would otherwise pass every bound.
func validReportSpeed(speed float64) bool {
	return !math.IsNaN(speed) && speed > 0 && speed <= maxReportSpeedMPS
}
