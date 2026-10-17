package api

import (
	"net/http"
	"strings"

	"fleetflow/internal/trip"
)

// defaultTripStatus is the status a create leaves out. DESIGN 3.5 says a created
// trip is planned; the status token is not optional to the domain, so the
// transport supplies the default rather than making every client spell it out.
const defaultTripStatus = "planned"

// tripView is a trip as the API writes it. VehicleID is left out when the trip
// has no vehicle, so "unassigned" reads the same as "absent".
type tripView struct {
	ID          string `json:"id"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	VehicleID   string `json:"vehicle_id,omitempty"`
	Status      string `json:"status"`
}

// viewOfTrip renders a stored trip for a response.
func viewOfTrip(t trip.Trip) tripView {
	return tripView{
		ID:          t.ID,
		Origin:      t.Origin,
		Destination: t.Destination,
		VehicleID:   t.VehicleID,
		Status:      t.Status.String(),
	}
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
