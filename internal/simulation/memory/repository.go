package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"fleetflow/internal/domain"
	"fleetflow/internal/simulation"
	"fleetflow/internal/simulation/sumo"
)

type Repository struct {
	mu   sync.RWMutex
	runs map[string]simulation.Run
}

func New() *Repository {
	return &Repository{runs: make(map[string]simulation.Run)}
}

func (r *Repository) Create(ctx context.Context, run simulation.Run) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.runs[run.ID]; exists {
		return domain.Conflict("simulation_run_exists", "simulation run %q already exists", run.ID)
	}
	for _, existing := range r.runs {
		if existing.Status == simulation.StatusQueued || existing.Status == simulation.StatusRunning {
			return domain.Conflict(simulation.CodeRunConflict, "a SUMO scenario is already running")
		}
	}
	r.runs[run.ID] = clone(run)
	return nil
}

func (r *Repository) ActiveRun(ctx context.Context) (*simulation.Run, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, run := range r.runs {
		if run.Status == simulation.StatusQueued || run.Status == simulation.StatusRunning {
			cloned := clone(run)
			return &cloned, nil
		}
	}
	return nil, nil
}

func (r *Repository) RecoverInterrupted(ctx context.Context, reason string) ([]simulation.Run, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	var recovered []simulation.Run
	for id, run := range r.runs {
		if run.Status == simulation.StatusQueued || run.Status == simulation.StatusRunning {
			run.Status = simulation.StatusFailed
			if run.Error != "" {
				run.Error = run.Error + "; " + reason
			} else {
				run.Error = reason
			}
			run.FinishedAt = &now
			r.runs[id] = run
			recovered = append(recovered, clone(run))
		}
	}
	return recovered, nil
}

func (r *Repository) Update(ctx context.Context, run simulation.Run) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.runs[run.ID]; !exists {
		return simulation.ErrRunNotFound
	}
	r.runs[run.ID] = clone(run)
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (simulation.Run, error) {
	if err := ctx.Err(); err != nil {
		return simulation.Run{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, exists := r.runs[id]
	if !exists {
		return simulation.Run{}, simulation.ErrRunNotFound
	}
	return clone(run), nil
}

func (r *Repository) List(ctx context.Context) ([]simulation.Run, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	runs := make([]simulation.Run, 0, len(r.runs))
	for _, run := range r.runs {
		record := clone(run)
		record.Result = nil
		runs = append(runs, record)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].CreatedAt.Equal(runs[j].CreatedAt) {
			return runs[i].ID > runs[j].ID
		}
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	return runs, nil
}

func clone(run simulation.Run) simulation.Run {
	if run.StartedAt != nil {
		started := *run.StartedAt
		run.StartedAt = &started
	}
	if run.FinishedAt != nil {
		finished := *run.FinishedAt
		run.FinishedAt = &finished
	}
	if run.Result == nil {
		return run
	}
	result := *run.Result
	result.Trips = append([]sumo.Trip(nil), result.Trips...)
	result.Tracks = make([]sumo.Track, len(run.Result.Tracks))
	for i, track := range run.Result.Tracks {
		result.Tracks[i] = sumo.Track{
			VehicleID: track.VehicleID,
			Positions: append([]sumo.Position(nil), track.Positions...),
		}
	}
	result.Deliveries = make([]sumo.Delivery, len(run.Result.Deliveries))
	for i, delivery := range run.Result.Deliveries {
		result.Deliveries[i] = sumo.Delivery{
			ID: delivery.ID, TripID: delivery.TripID, VehicleID: delivery.VehicleID,
			Status:   delivery.Status,
			Packages: append([]sumo.Package(nil), delivery.Packages...),
			Stops:    append([]sumo.DeliveryStop(nil), delivery.Stops...),
			Events:   append([]sumo.DeliveryEvent(nil), delivery.Events...),
		}
	}
	run.Result = &result
	return run
}
