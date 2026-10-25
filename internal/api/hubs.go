package api

import (
	"net/http"

	"fleetflow/internal/simulation"
)

// handleListHubs returns the logistics hubs the scenario generator routes
// between.
//
// The map draws these places and the generator names them as trip origins and
// destinations, so the catalogue is served from the generator's own list. That
// is what keeps the two in step: a hub a scenario drives to cannot be missing
// from the map, and the map cannot draw a hub no scenario uses. A copy of the
// list in the console had drifted and left Soekarno-Hatta Cargo Terminal as an
// unmarked place where vehicles start.
func (s *Server) handleListHubs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"hubs": simulation.JakartaHubs})
}
