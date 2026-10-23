package api

import (
	"net/http"
	"strings"
	"time"

	"fleetflow/internal/domain"
)

const (
	maxLocationSpeedMPS  = 90.0
	maxLocationClockSkew = 5 * time.Minute
)

type locationRequest struct {
	VehicleID string   `json:"vehicle_id"`
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	SpeedMPS  *float64 `json:"speed_mps,omitempty"`
	TS        *int64   `json:"ts,omitempty"`
}

type vehicleLocation struct {
	VehicleID string    `json:"vehicle_id"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	SpeedMPS  float64   `json:"speed_mps"`
	UpdatedAt time.Time `json:"updated_at"`
	AgeS      float64   `json:"age_s"`
}

func (s *Server) handleCreateLocation(w http.ResponseWriter, r *http.Request) {
	var req locationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	req.VehicleID = strings.TrimSpace(req.VehicleID)
	if req.VehicleID == "" {
		writeError(w, r, domain.Invalid("vehicle_id_required", "vehicle_id is required"))
		return
	}
	if req.Lat < -90 || req.Lat > 90 || req.Lon < -180 || req.Lon > 180 {
		writeError(w, r, domain.Invalid("location_out_of_range", "lat and lon must be valid geographic coordinates"))
		return
	}

	speed := 0.0
	if req.SpeedMPS != nil {
		speed = *req.SpeedMPS
		if speed < 0 || speed > maxLocationSpeedMPS {
			writeError(w, r, domain.Invalid("speed_out_of_range",
				"speed_mps must be between 0 and %g", maxLocationSpeedMPS))
			return
		}
	}

	observedAt := time.Now()
	if req.TS != nil {
		if *req.TS <= 0 {
			writeError(w, r, domain.Invalid("ts_invalid", "ts must be a Unix time in milliseconds"))
			return
		}
		observedAt = time.UnixMilli(*req.TS)
		now := time.Now()
		if observedAt.After(now.Add(maxLocationClockSkew)) {
			writeError(w, r, domain.Invalid("ts_in_future",
				"ts must not be more than %s ahead of this server's clock", maxLocationClockSkew))
			return
		}
		if now.Sub(observedAt) > liveVehicleTTL {
			writeError(w, r, domain.Invalid("location_stale",
				"location timestamp is older than the %s live-position window", liveVehicleTTL))
			return
		}
	}

	s.live.observePosition(req.VehicleID, req.Lat, req.Lon, speed, observedAt)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *Server) handleListLocations(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	seen := s.live.snapshot(now)
	positions := make([]vehicleLocation, 0, len(seen))
	for _, vehicle := range seen {
		positions = append(positions, vehicleLocation{
			VehicleID: vehicle.ID,
			Lat:       vehicle.Lat,
			Lon:       vehicle.Lon,
			SpeedMPS:  vehicle.SpeedMPS,
			UpdatedAt: vehicle.At.UTC(),
			AgeS:      now.Sub(vehicle.At).Seconds(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"positions": positions})
}
