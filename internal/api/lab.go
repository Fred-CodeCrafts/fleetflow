package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"fleetflow/internal/domain"
	"fleetflow/internal/lab"
)

// The routing lab's bounds. They are tighter than the lab package's own limits
// because this endpoint answers a browser request inside the server's timeout:
// a sweep at the top of these bounds still finishes in about a second.
const (
	labMaxSeeds    = 25
	labMaxWaves    = 60
	labMaxVehicles = 400
	labMaxGrid     = 12
)

// LabDynamicResponse is one seeded static-versus-dynamic experiment.
type LabDynamicResponse struct {
	Config     LabDynamicConfig `json:"config"`
	Summary    lab.Summary      `json:"summary"`
	Method     LabMethod        `json:"method"`
	DurationMS int64            `json:"duration_ms"`
}

// LabDynamicConfig echoes what was run, so a reader can repeat it exactly.
type LabDynamicConfig struct {
	Seeds    int     `json:"seeds"`
	Share    float64 `json:"share"`
	Waves    int     `json:"waves"`
	Vehicles int     `json:"vehicles"`
	Grid     int     `json:"grid"`
	BaseSeed int64   `json:"base_seed"`
}

// LabMethod names the models behind the numbers, in the same spirit as the
// emissions provenance: a figure that does not say what produced it is not a
// result.
type LabMethod struct {
	Model       string `json:"model"`
	Network     string `json:"network"`
	Routing     string `json:"routing"`
	Congestion  string `json:"congestion"`
	Assumptions string `json:"assumptions"`
	NotIncluded string `json:"not_included"`
}

// handleLabDynamic runs the seeded comparison and answers with the summary.
//
// It is public and read-only: the experiment is computed in memory on every
// request from the seed alone, so nothing is written and two identical requests
// produce identical numbers.
func (s *Server) handleLabDynamic(w http.ResponseWriter, r *http.Request) {
	cfg, seeds, err := labConfigFromQuery(r)
	if err != nil {
		writeError(w, r, domain.Invalid("invalid_lab_parameter", "%s", err.Error()))
		return
	}

	start := time.Now()
	summary, err := lab.Sweep(cfg, seeds)
	if err != nil {
		if errors.Is(err, lab.ErrInvalidConfig) {
			writeError(w, r, domain.Invalid("invalid_lab_parameter", "%s", err.Error()))
			return
		}
		writeError(w, r, domain.Internal("lab_failed", "the experiment could not be run"))
		return
	}

	writeJSON(w, http.StatusOK, LabDynamicResponse{
		Config: LabDynamicConfig{
			Seeds:    seeds,
			Share:    cfg.Share,
			Waves:    cfg.Waves,
			Vehicles: cfg.Vehicles,
			Grid:     cfg.Network.Size,
			BaseSeed: cfg.Seed,
		},
		Summary:    summary,
		Method:     labMethodFor(cfg),
		DurationMS: time.Since(start).Milliseconds(),
	})
}

func labConfigFromQuery(r *http.Request) (lab.Config, int, error) {
	q := r.URL.Query()
	cfg := lab.DefaultConfig()

	seeds, err := intParam(q.Get("seeds"), 5, 1, labMaxSeeds, "seeds")
	if err != nil {
		return cfg, 0, err
	}
	waves, err := intParam(q.Get("waves"), cfg.Waves, 1, labMaxWaves, "waves")
	if err != nil {
		return cfg, 0, err
	}
	vehicles, err := intParam(q.Get("vehicles"), cfg.Vehicles, 1, labMaxVehicles, "vehicles")
	if err != nil {
		return cfg, 0, err
	}
	grid, err := intParam(q.Get("grid"), cfg.Network.Size, 2, labMaxGrid, "grid")
	if err != nil {
		return cfg, 0, err
	}
	share, err := floatParam(q.Get("share"), cfg.Share)
	if err != nil {
		return cfg, 0, err
	}

	cfg.Waves = waves
	cfg.Vehicles = vehicles
	cfg.Share = share
	cfg.Network.Size = grid
	return cfg, seeds, nil
}

func intParam(raw string, def, min, max int, name string) (int, error) {
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a whole number, got %q", name, raw)
	}
	if v < min || v > max {
		return 0, fmt.Errorf("%s must be between %d and %d, got %d", name, min, max, v)
	}
	return v, nil
}

func floatParam(raw string, def float64) (float64, error) {
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("share must be a number, got %q", raw)
	}
	if v < 0 || v > 1 {
		return 0, fmt.Errorf("share must be between 0 and 1, got %g", v)
	}
	return v, nil
}

func labMethodFor(cfg lab.Config) LabMethod {
	return LabMethod{
		Model:       "FleetFlow dynamic edge weights v1 (lab)",
		Network:     fmt.Sprintf("%dx%d lattice, %.0f m streets; arterials every %d rows and columns at %.0f m and %.1f km/h", cfg.Network.Size, cfg.Network.Size, cfg.Network.SpacingM, cfg.Network.ArterialEvery, cfg.Network.ArterialLengthM, cfg.Network.ArterialSpeedMPS*3.6),
		Routing:     "Dijkstra over an immutable metric snapshot; the static arm always costs edges at free-flow time",
		Congestion:  "each extra simultaneous vehicle on an edge adds 25% to its travel time, capped at four times free flow",
		Assumptions: "dynamic arm smooths reported congestion with an EMA (gamma 0.3), decays quiet edges toward free flow (lambda 0.1), and gives each driver a 35% private perception error; every simulated vehicle reports the speed it achieved",
		NotIncluded: "not the thesis: no CCH, no TraCI, no LuST scenario, no Jakarta street plan, no measured traffic",
	}
}
