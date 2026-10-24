package api

import (
	"errors"
	"net/http"

	"fleetflow/internal/domain"
	"fleetflow/internal/osrm"
)

// routeRequest is the body of POST /api/route.
type routeRequest struct {
	From osrm.Coordinate `json:"from"`
	To   osrm.Coordinate `json:"to"`
}

type routeResponse struct {
	Geometry  [][]float64 `json:"geometry"`
	DistanceM float64     `json:"distance_m"`
	DurationS float64     `json:"duration_s"`
	Provider  string      `json:"provider"`
}

// handleRoute delegates all pathfinding to the configured OSRM service.
func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	var req routeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if !validRouteCoordinate(req.From) || !validRouteCoordinate(req.To) {
		writeError(w, r, domain.Invalid("route_coordinates_invalid",
			"from and to must contain valid latitude and longitude coordinates"))
		return
	}
	if s.router == nil {
		writeError(w, r, domain.Unavailable("routing_unavailable",
			"OSRM routing is not configured; set OSRM_URL and start the routing service"))
		return
	}

	route, err := s.router.Route(r.Context(), req.From, req.To)
	if err != nil {
		if errors.Is(err, osrm.ErrNoRoute) {
			writeError(w, r, domain.NotFound("route_not_found", "OSRM could not find a driving route between those locations"))
			return
		}
		writeError(w, r, domain.Unavailable("routing_unavailable",
			"OSRM could not calculate a route; check that the routing service and Jakarta dataset are available"))
		return
	}

	writeJSON(w, http.StatusOK, routeResponse{
		Geometry:  route.Geometry,
		DistanceM: route.DistanceM,
		DurationS: route.DurationS,
		Provider:  "OSRM",
	})
}

func validRouteCoordinate(point osrm.Coordinate) bool {
	return point.Lat >= -90 && point.Lat <= 90 && point.Lon >= -180 && point.Lon <= 180
}
