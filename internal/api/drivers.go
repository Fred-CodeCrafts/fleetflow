package api

import (
	"net/http"

	"fleetflow/internal/driver"
)

// driverView is a driver as the API writes it. VehicleID is left out when the
// driver holds no vehicle, so "unassigned" reads the same as "absent".
type driverView struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Contact       string  `json:"contact"`
	ContactPhone  *string `json:"contact_phone"`
	LicenseClass  *string `json:"license_class"`
	LicenseExpiry *string `json:"license_expiry"`
	ShiftStart    *string `json:"shift_start"`
	ShiftEnd      *string `json:"shift_end"`
	Availability  *string `json:"availability"`
	VehicleID     string  `json:"vehicle_id,omitempty"`
}

// viewOfDriver renders a stored driver for a response.
func viewOfDriver(d driver.Driver) driverView {
	return driverView{
		ID: d.ID, Name: d.Name, Contact: d.Contact, ContactPhone: optionalText(d.ContactPhone),
		LicenseClass: optionalText(d.LicenseClass), LicenseExpiry: optionalText(d.LicenseExpiry),
		ShiftStart: optionalText(d.ShiftStart), ShiftEnd: optionalText(d.ShiftEnd),
		Availability: optionalText(d.Availability), VehicleID: d.VehicleID,
	}
}

// createDriverRequest is the body of POST /api/drivers (DESIGN 3.5).
type createDriverRequest struct {
	Name          string `json:"name"`
	Contact       string `json:"contact"`
	ContactPhone  string `json:"contact_phone"`
	LicenseClass  string `json:"license_class"`
	LicenseExpiry string `json:"license_expiry"`
	ShiftStart    string `json:"shift_start"`
	ShiftEnd      string `json:"shift_end"`
	Availability  string `json:"availability"`
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

	created, err := s.drivers.Create(r.Context(), driver.CreateInput{
		Name: req.Name, Contact: req.Contact, ContactPhone: req.ContactPhone,
		LicenseClass: req.LicenseClass, LicenseExpiry: req.LicenseExpiry,
		ShiftStart: req.ShiftStart, ShiftEnd: req.ShiftEnd, Availability: req.Availability,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/drivers/"+created.ID)
	writeJSON(w, http.StatusCreated, viewOfDriver(created))
}
