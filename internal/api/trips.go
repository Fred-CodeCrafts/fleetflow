package api

import (
	"net/http"
	"time"

	"fleetflow/internal/trip"
)

// tripView renders a stored trip for the JSON API.
type tripView struct {
	ID          string     `json:"id"`
	Origin      string     `json:"origin"`
	Destination string     `json:"destination"`
	VehicleID   string     `json:"vehicle_id,omitempty"`
	Status      string     `json:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	DurationS   *float64   `json:"duration_s,omitempty"`
}

func viewOfTrip(t trip.Trip) tripView {
	v := tripView{
		ID:          t.ID,
		Origin:      t.Origin,
		Destination: t.Destination,
		VehicleID:   t.VehicleID,
		Status:      t.Status.String(),
	}
	if !t.StartedAt.IsZero() {
		v.StartedAt = &t.StartedAt
	}
	if !t.FinishedAt.IsZero() {
		v.FinishedAt = &t.FinishedAt
	}
	if dur, ok := t.Duration(); ok {
		s := dur.Seconds()
		v.DurationS = &s
	}
	return v
}

type createTripRequest struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	VehicleID   string `json:"vehicle_id"`
	Status      string `json:"status"`
}

type updateTripStatusRequest struct {
	Status string `json:"status"`
}

func (s *Server) handleListTrips(w http.ResponseWriter, r *http.Request) {
	if s.trips == nil {
		writeError(w, r, errNoFleet)
		return
	}

	trips, err := s.trips.List(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	views := make([]tripView, 0, len(trips))
	for _, t := range trips {
		views = append(views, viewOfTrip(t))
	}
	writeJSON(w, http.StatusOK, map[string][]tripView{"trips": views})
}

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

	status := req.Status
	if status == "" {
		status = "planned"
	}

	created, err := s.trips.Create(r.Context(), trip.CreateInput{
		Origin:      req.Origin,
		Destination: req.Destination,
		VehicleID:   req.VehicleID,
		Status:      status,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, viewOfTrip(created))
}

func (s *Server) handleUpdateTripStatus(w http.ResponseWriter, r *http.Request) {
	if s.trips == nil {
		writeError(w, r, errNoFleet)
		return
	}

	id := r.PathValue("id")
	var req updateTripStatusRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	updated, err := s.trips.SetStatus(r.Context(), id, req.Status)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, viewOfTrip(updated))
}
