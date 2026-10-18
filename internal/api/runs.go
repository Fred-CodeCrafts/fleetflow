package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"fleetflow/internal/domain"
	runstore "fleetflow/internal/run"
	"fleetflow/internal/sim"
)

// codeLimitUnreadable is reported for a limit query parameter that is not a whole
// number. The client asked for something this endpoint cannot read, which is a
// different answer from "the default page".
const codeLimitUnreadable = "limit_unreadable"

// codeNoRun is reported by a save while no comparison run is going. There is
// nothing to save, and a record of nothing would read later as a run that
// produced nothing.
const codeNoRun = "no_run"

// codeBodyNotTaken is reported for a save request that carries a body. The run is
// captured from the server, so a body would be read by nobody: refusing it says
// so, where ignoring it would let a client believe it had chosen what to save.
const codeBodyNotTaken = "body_not_taken"

// runView is a saved run as the API writes it. The two documents travel as the
// JSON they were stored as, embedded rather than re-encoded from a struct: the
// API that produced them owns their shape, and re-encoding here would silently
// drop any field this layer did not know about.
//
// A document read back is the stored form, not necessarily the bytes that were
// written: Postgres jsonb canonicalizes (key order, whitespace, number spelling),
// so a round trip is equal as JSON and not byte for byte. Nothing here compares
// the two, and a client should not either.
type runView struct {
	ID   string `json:"id"`
	Seed int64  `json:"seed"`
	// Config is the demand the run was started with, in the shape
	// POST /api/sim/start accepts.
	Config json.RawMessage `json:"config"`
	// Metrics is the run's own metrics document, the object GET /api/metrics
	// reports under "run": the engine's figures are about the process, not about
	// this run, so they are not kept with it.
	Metrics json.RawMessage `json:"metrics"`
	// CreatedAt is when the run was saved. A run that is saved again is a second
	// record: nothing here rewrites one that already exists.
	CreatedAt time.Time `json:"created_at"`
}

// viewOfRun renders a stored run for a response.
func viewOfRun(saved runstore.Run) runView {
	return runView{
		ID:        saved.ID,
		Seed:      saved.Seed,
		Config:    json.RawMessage(saved.Config),
		Metrics:   json.RawMessage(saved.Metrics),
		CreatedAt: saved.CreatedAt.UTC(),
	}
}

// runConfigDocument is the demand a saved run was started with. It carries every
// field of sim.Config, under the same wire names POST /api/sim/start accepts, so
// a stored config can be posted straight back and start the same run again --
// which is what makes a saved run reproducible rather than merely descriptive.
type runConfigDocument struct {
	Seed             int64   `json:"seed"`
	Vehicles         int     `json:"vehicles"`
	FleetShare       float64 `json:"fleet_share"`
	StepS            float64 `json:"step_s"`
	WindowS          float64 `json:"window_s"`
	RerouteEveryS    float64 `json:"reroute_every_s"`
	RerouteThreshold float64 `json:"reroute_threshold"`
}

// configOf copies a run's configuration into the stored document.
func configOf(cfg sim.Config) runConfigDocument {
	return runConfigDocument{
		Seed:             cfg.Seed,
		Vehicles:         cfg.Vehicles,
		FleetShare:       cfg.FleetShare,
		StepS:            cfg.StepS,
		WindowS:          cfg.WindowS,
		RerouteEveryS:    cfg.RerouteEveryS,
		RerouteThreshold: cfg.RerouteThreshold,
	}
}

// handleListRuns answers GET /api/runs with the saved runs, most recent first. A
// limit query parameter chooses the page size, within what the service allows; a
// limit it cannot read is refused rather than quietly replaced by the default,
// because the client asked for a page it would not get.
func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	if s.runs == nil {
		writeError(w, r, errNoRuns)
		return
	}

	limit, err := runsLimit(r)
	if err != nil {
		writeError(w, r, err)
		return
	}

	found, err := s.runs.List(r.Context(), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}

	views := make([]runView, 0, len(found))
	for _, saved := range found {
		views = append(views, viewOfRun(saved))
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": views})
}

// runsLimit reads the optional limit query parameter. An absent one is the zero
// the service already understands as "the default page", so the page size is
// decided in one place and this layer only refuses what it cannot read.
func runsLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, domain.Invalid(codeLimitUnreadable, "limit must be a whole number")
	}
	return limit, nil
}

// handleSaveRun answers POST /api/runs by storing the comparison run that is
// going, as the API itself serializes it (DESIGN 3.5, PRD US-9).
//
// The record is captured from the server rather than posted by the client, so
// what is saved is, by construction, what ran: a client-supplied pair of
// documents could describe a run that never happened, or pair one run's demand
// with another's results, and the record would carry no sign of it. The endpoint
// therefore takes no body, and says so rather than ignoring one.
//
// A run that has not settled is saved as it stands. The metrics count the trips
// that finished, so a partial record does not read as a complete one; refusing
// instead would tie the endpoint to a clock that parks while nobody is watching
// (T44), and a run watched to the end is exactly the case worth keeping.
func (s *Server) handleSaveRun(w http.ResponseWriter, r *http.Request) {
	if s.runs == nil {
		writeError(w, r, errNoRuns)
		return
	}
	if r.ContentLength != 0 {
		writeError(w, r, domain.Invalid(codeBodyNotTaken,
			"this endpoint takes no body: the run it saves is the one the server is running"))
		return
	}

	live := s.currentRun()
	if live == nil {
		writeError(w, r, domain.Conflict(codeNoRun, "no comparison run is going: start one before saving it"))
		return
	}

	// Both documents are built here, from the run itself, so the record cannot
	// disagree with what the run was doing at the moment it was saved. A failed
	// marshal is impossible for these types, and is still reported rather than
	// stored as a zero value.
	config, err := json.Marshal(configOf(live.cfg))
	if err != nil {
		writeError(w, r, err)
		return
	}
	metrics, err := json.Marshal(live.metrics())
	if err != nil {
		writeError(w, r, err)
		return
	}

	saved, err := s.runs.Save(r.Context(), live.cfg.Seed, config, metrics)
	if err != nil {
		writeError(w, r, err)
		return
	}

	// No Location header: DESIGN gives no GET /api/runs/{id}, and pointing at a
	// route that does not exist would be a claim the server cannot honour.
	writeJSON(w, http.StatusCreated, viewOfRun(saved))
}
