package api

import (
	"context"
	"net/http"

	"fleetflow/internal/domain"
	"fleetflow/internal/driver"
)

// driverView is a driver as the API writes it. VehicleID is left out when the
// driver holds no vehicle, so "unassigned" reads the same as "absent".
type driverView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Contact   string `json:"contact"`
	VehicleID string `json:"vehicle_id,omitempty"`
}

// viewOfDriver renders a stored driver for a response.
func viewOfDriver(d driver.Driver) driverView {
	return driverView{ID: d.ID, Name: d.Name, Contact: d.Contact, VehicleID: d.VehicleID}
}

// createDriverRequest is the body of POST /api/drivers (DESIGN 3.5).
type createDriverRequest struct {
	Name    string `json:"name"`
	Contact string `json:"contact"`
}

// assignDriverRequest is the body of POST /api/vehicles/{id}/driver.
//
// A blank driver_id takes the driver currently on the vehicle off it. The
// endpoint is addressed by vehicle, so a client that wants a vehicle emptied
// should not have to look up who is on it first; the blank id is how it says
// "whichever driver is there".
type assignDriverRequest struct {
	DriverID string `json:"driver_id"`
}

// assignDriverResponse reports what the assignment left behind, so a client that
// sent a blank driver_id learns which driver it took off. Driver is null when the
// vehicle ends up with nobody on it.
type assignDriverResponse struct {
	VehicleID string      `json:"vehicle_id"`
	Driver    *driverView `json:"driver"`
}

// handleListDrivers answers GET /api/drivers.
func (s *Server) handleListDrivers(w http.ResponseWriter, r *http.Request) {
	if s.drivers == nil {
		writeError(w, r, errNoFleet)
		return
	}

	found, err := s.drivers.List(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	views := make([]driverView, 0, len(found))
	for _, d := range found {
		views = append(views, viewOfDriver(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"drivers": views})
}

// handleCreateDriver answers POST /api/drivers with the stored record.
func (s *Server) handleCreateDriver(w http.ResponseWriter, r *http.Request) {
	if s.drivers == nil {
		writeError(w, r, errNoFleet)
		return
	}

	var req createDriverRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	created, err := s.drivers.Create(r.Context(), driver.CreateInput{Name: req.Name, Contact: req.Contact})
	if err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/drivers/"+created.ID)
	writeJSON(w, http.StatusCreated, viewOfDriver(created))
}

// handleAssignDriver answers POST /api/vehicles/{id}/driver: it puts a driver on
// the vehicle, or takes the driver currently on it off.
//
// The rule that a vehicle belongs to one driver is the driver service's, and
// behind it the database's. This handler only decides which driver the request
// names.
func (s *Server) handleAssignDriver(w http.ResponseWriter, r *http.Request) {
	if s.drivers == nil {
		writeError(w, r, errNoFleet)
		return
	}

	vehicleID := r.PathValue("id")
	if vehicleID == "" {
		writeError(w, r, domain.Invalid(codeVehicleIDRequired, "a vehicle id is required in the path"))
		return
	}

	var req assignDriverRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	if req.DriverID != "" {
		updated, err := s.drivers.Assign(r.Context(), req.DriverID, vehicleID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		view := viewOfDriver(updated)
		writeJSON(w, http.StatusOK, assignDriverResponse{VehicleID: vehicleID, Driver: &view})
		return
	}

	// Unassigning: the driver to take off is the one holding this vehicle. That is
	// a read before the write, which is safe here because the write names the
	// driver that was read: a driver who moved in the meantime keeps the vehicle
	// it moved to, and is not disturbed.
	current, err := s.driverOnVehicle(r.Context(), vehicleID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if current == nil {
		// Already unassigned. Nothing to do, and nothing for the client to fix.
		writeJSON(w, http.StatusOK, assignDriverResponse{VehicleID: vehicleID})
		return
	}

	if _, err := s.drivers.Assign(r.Context(), current.ID, ""); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, assignDriverResponse{VehicleID: vehicleID})
}

// driverOnVehicle returns the driver holding vehicleID, or nil when none does.
func (s *Server) driverOnVehicle(ctx context.Context, vehicleID string) (*driver.Driver, error) {
	all, err := s.drivers.List(ctx)
	if err != nil {
		return nil, err
	}
	for i, d := range all {
		if d.VehicleID == vehicleID {
			return &all[i], nil
		}
	}
	return nil, nil
}
