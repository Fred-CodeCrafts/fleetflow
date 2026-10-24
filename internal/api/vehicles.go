package api

import (
	"net/http"
	"strings"

	"fleetflow/internal/vehicle"
)

// vehicleView is a vehicle as the API writes it. The domain type carries no JSON
// tags on purpose — it is the stored record, not a wire format — so the handler
// builds its own view, and the status travels as the wire name the create
// endpoint accepts rather than as an opaque number.
type vehicleView struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Type               string   `json:"type"`
	Status             string   `json:"status"`
	Registration       *string  `json:"registration"`
	Make               *string  `json:"make"`
	Model              *string  `json:"model"`
	Year               *int     `json:"year"`
	Color              *string  `json:"color"`
	BodyType           *string  `json:"body_type"`
	FuelType           *string  `json:"fuel_type"`
	Capacity           *float64 `json:"capacity"`
	MaintenanceHistory *string  `json:"maintenance_history"`
}

// viewOfVehicle renders a stored vehicle for a response.
func viewOfVehicle(v vehicle.Vehicle) vehicleView {
	return vehicleView{
		ID: v.ID, Name: v.Name, Type: v.Type, Status: v.Status.String(),
		Registration: optionalText(v.Registration), Make: optionalText(v.Make), Model: optionalText(v.Model),
		Year: v.Year, Color: optionalText(v.Color), BodyType: optionalText(v.BodyType),
		FuelType: optionalText(v.FuelType), Capacity: v.Capacity,
		MaintenanceHistory: optionalText(v.MaintenanceHistory),
	}
}

// createVehicleRequest is the body of POST /api/vehicles (DESIGN 3.5). Name holds
// a name or a plate, since the PRD accepts either.
type createVehicleRequest struct {
	Name               string   `json:"name"`
	Type               string   `json:"type"`
	Status             string   `json:"status"`
	Registration       string   `json:"registration"`
	Make               string   `json:"make"`
	Model              string   `json:"model"`
	Year               *int     `json:"year"`
	Color              string   `json:"color"`
	BodyType           string   `json:"body_type"`
	FuelType           string   `json:"fuel_type"`
	Capacity           *float64 `json:"capacity"`
	MaintenanceHistory string   `json:"maintenance_history"`
}

// handleListVehicles answers GET /api/vehicles, optionally narrowed by a status
// query parameter.
func (s *Server) handleListVehicles(w http.ResponseWriter, r *http.Request) {
	if s.vehicles == nil {
		writeError(w, r, errNoFleet)
		return
	}

	filter, err := vehicleFilter(r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, r, err)
		return
	}

	found, err := s.vehicles.List(r.Context(), filter)
	if err != nil {
		writeError(w, r, err)
		return
	}

	views := make([]vehicleView, 0, len(found))
	for _, v := range found {
		views = append(views, viewOfVehicle(v))
	}
	writeJSON(w, http.StatusOK, map[string]any{"vehicles": views})
}

// handleCreateVehicle answers POST /api/vehicles with the stored record, which is
// where its generated id comes from.
func (s *Server) handleCreateVehicle(w http.ResponseWriter, r *http.Request) {
	if s.vehicles == nil {
		writeError(w, r, errNoFleet)
		return
	}

	var req createVehicleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	created, err := s.vehicles.Create(r.Context(), vehicle.CreateInput{
		Name: req.Name, Type: req.Type, Status: req.Status,
		Registration: req.Registration, Make: req.Make, Model: req.Model, Year: req.Year,
		Color: req.Color, BodyType: req.BodyType, FuelType: req.FuelType,
		Capacity: req.Capacity, MaintenanceHistory: req.MaintenanceHistory,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/vehicles/"+created.ID)
	writeJSON(w, http.StatusCreated, viewOfVehicle(created))
}

func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// vehicleFilter turns the optional status parameter into a filter. An absent or
// blank value lists every vehicle; anything else must name a known status, so a
// typo is reported instead of quietly listing everything.
func vehicleFilter(status string) (vehicle.ListFilter, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		return vehicle.ListFilter{}, nil
	}
	parsed, err := vehicle.ParseStatus(status)
	if err != nil {
		return vehicle.ListFilter{}, err
	}
	return vehicle.ByStatus(parsed), nil
}
