package api

import (
	"net/http"
	"strings"
	"time"

	"fleetflow/internal/domain"
	"fleetflow/internal/trip"
)

// defaultTripStatus is the status a create leaves out. DESIGN 3.5 says a created
// trip is planned; the status token is not optional to the domain, so the
// transport supplies the default rather than making every client spell it out.
const defaultTripStatus = "planned"

// codeTripIDRequired is reported when a trip endpoint is reached without a trip
// id in the path.
const codeTripIDRequired = "trip_id_required"

// tripView is a trip as the API writes it. VehicleID is left out when the trip
// has no vehicle, so "unassigned" reads the same as "absent".
//
// StartedAt, FinishedAt and DurationS are pointers on purpose. A trip that has
// not started has no start time, and a trip that never recorded one has no
// duration; reporting 0 or an epoch date would be a claim the data does not
// support (PRD US-9). Null is the honest answer, and the console renders it as
// "not recorded".
type tripView struct {
	ID          string   `json:"id"`
	Origin      string   `json:"origin"`
	Destination string   `json:"destination"`
	VehicleID   string   `json:"vehicle_id,omitempty"`
	Status      string   `json:"status"`
	StartedAt   *string  `json:"started_at"`
	FinishedAt  *string  `json:"finished_at"`
	DurationS   *float64 `json:"duration_s"`
}

// viewOfTrip renders a stored trip for a response.
func viewOfTrip(t trip.Trip) tripView {
	view := tripView{
		ID:          t.ID,
		Origin:      t.Origin,
		Destination: t.Destination,
		VehicleID:   t.VehicleID,
		Status:      t.Status.String(),
	}
	if !t.StartedAt.IsZero() {
		at := t.StartedAt.UTC().Format(time.RFC3339Nano)
		view.StartedAt = &at
	}
	if !t.FinishedAt.IsZero() {
		at := t.FinishedAt.UTC().Format(time.RFC3339Nano)
		view.FinishedAt = &at
	}
	if d, ok := t.Duration(); ok {
		seconds := d.Seconds()
		view.DurationS = &seconds
	}
	return view
}

// createTripRequest is the body of POST /api/trips (DESIGN 3.5).
type createTripRequest struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	VehicleID   string `json:"vehicle_id"`
	Status      string `json:"status"`
}

// statusOrDefault returns the status the request asks for, or planned when it
// asks for none.
func (r createTripRequest) statusOrDefault() string {
	if strings.TrimSpace(r.Status) == "" {
		return defaultTripStatus
	}
	return r.Status
}

// handleListTrips answers GET /api/trips.
func (s *Server) handleListTrips(w http.ResponseWriter, r *http.Request) {
	if s.trips == nil {
		writeError(w, r, errNoFleet)
		return
	}

	found, err := s.trips.List(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	views := make([]tripView, 0, len(found))
	for _, t := range found {
		views = append(views, viewOfTrip(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"trips": views})
}

// handleCreateTrip answers POST /api/trips with the stored record.
//
// A trip that names a vehicle is an assignment, so it goes through the same
// rules as any other: the vehicle must exist and be available, and a vehicle
// cannot end up on two unfinished trips. Those rules are the trip service's, and
// behind it the database's, rather than something this handler checks and then
// hopes still holds when it writes.
func (s *Server) handleCreateTrip(w http.ResponseWriter, r *http.Request) {
	if s.trips == nil {
		writeError(w, r, errNoFleet)
		return
	}

	var req createTripRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	created, err := s.trips.Create(r.Context(), trip.CreateInput{
		Origin:      req.Origin,
		Destination: req.Destination,
		VehicleID:   req.VehicleID,
		Status:      req.statusOrDefault(),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/trips/"+created.ID)
	writeJSON(w, http.StatusCreated, viewOfTrip(created))
}

// setTripStatusRequest is the body of POST /api/trips/{id}/status (DESIGN 3.5).
// The status is required: unlike a create, there is no sensible default for a
// move, and a blank one is a client mistake rather than "whatever comes next".
type setTripStatusRequest struct {
	Status string `json:"status"`
}

// handleSetTripStatus answers POST /api/trips/{id}/status, moving a trip along
// its life — planned, then in progress, then completed.
//
// Whether the move makes sense is the domain's to decide, and it decides it in
// the store, where the vehicle carrying the trip is moved with it. This handler
// only says which trip the request names.
func (s *Server) handleSetTripStatus(w http.ResponseWriter, r *http.Request) {
	if s.trips == nil {
		writeError(w, r, errNoFleet)
		return
	}

	tripID := r.PathValue("id")
	if tripID == "" {
		writeError(w, r, domain.Invalid(codeTripIDRequired, "a trip id is required in the path"))
		return
	}

	var req setTripStatusRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	updated, err := s.trips.SetStatus(r.Context(), tripID, req.Status)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOfTrip(updated))
}
