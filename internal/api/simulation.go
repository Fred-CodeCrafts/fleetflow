package api

import (
	"net/http"

	"fleetflow/internal/domain"
	"fleetflow/internal/simulation/sumo"
)

var errNoSimulation = domain.Unavailable("simulation_unavailable", "SUMO simulation is not configured")

type createSimulationRunRequest struct {
	Seed             int64 `json:"seed"`
	Vehicles         int   `json:"vehicles"`
	DepartureWindowS int   `json:"departure_window_s"`
}

func (s *Server) handleCreateSimulationRun(w http.ResponseWriter, r *http.Request) {
	if s.simulation == nil {
		writeError(w, r, errNoSimulation)
		return
	}

	var req createSimulationRunRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	run, err := s.simulation.Start(r.Context(), sumo.Scenario{
		Seed:             req.Seed,
		Vehicles:         req.Vehicles,
		DepartureWindowS: req.DepartureWindowS,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/simulation/runs/"+run.ID)
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) handleListSimulationRuns(w http.ResponseWriter, r *http.Request) {
	if s.simulation == nil {
		writeError(w, r, errNoSimulation)
		return
	}
	runs, err := s.simulation.List(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleGetSimulationRun(w http.ResponseWriter, r *http.Request) {
	if s.simulation == nil {
		writeError(w, r, errNoSimulation)
		return
	}
	run, err := s.simulation.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}
