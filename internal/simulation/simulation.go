package simulation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"fleetflow/internal/domain"
	"fleetflow/internal/simulation/sumo"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

const (
	CodeRunIDFailed    = "simulation_run_id_failed"
	CodeRunConflict    = "simulation_run_conflict"
	CodeRunUnavailable = "simulation_unavailable"
	CodeRunNotFound    = "simulation_run_not_found"

	codeRunIDFailed    = CodeRunIDFailed
	codeRunConflict    = CodeRunConflict
	codeRunUnavailable = CodeRunUnavailable
	codeRunNotFound    = CodeRunNotFound
)

type Run struct {
	ID         string        `json:"id"`
	Status     Status        `json:"status"`
	Scenario   sumo.Scenario `json:"scenario"`
	CreatedAt  time.Time     `json:"created_at"`
	StartedAt  *time.Time    `json:"started_at,omitempty"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
	Error      string        `json:"error,omitempty"`
	Result     *sumo.Result  `json:"result,omitempty"`
}

type Repository interface {
	Create(context.Context, Run) error
	Update(context.Context, Run) error
	Get(context.Context, string) (Run, error)
	List(context.Context) ([]Run, error)
	ActiveRun(context.Context) (*Run, error)
	RecoverInterrupted(context.Context, string) ([]Run, error)
}

type Executor interface {
	Run(context.Context, sumo.Scenario) (sumo.Result, error)
}

type Service struct {
	repo     Repository
	executor Executor
	logger   *slog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	active   bool
	closed   bool
	wg       sync.WaitGroup
}

func NewService(parent context.Context, repo Repository, executor Executor, logger *slog.Logger) (*Service, error) {
	if parent == nil {
		return nil, fmt.Errorf("simulation context is required")
	}
	if repo == nil {
		return nil, fmt.Errorf("simulation repository is required")
	}
	if executor == nil {
		return nil, fmt.Errorf("SUMO executor is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Service{repo: repo, executor: executor, logger: logger, ctx: ctx, cancel: cancel}
	if recovered, err := repo.RecoverInterrupted(ctx, "simulation was interrupted by server restart"); err != nil {
		logger.Warn("could not recover interrupted simulation runs on startup", "error", err)
	} else if len(recovered) > 0 {
		logger.Warn("recovered interrupted simulation runs", "count", len(recovered))
	}
	return s, nil
}

func (s *Service) Start(ctx context.Context, scenario sumo.Scenario) (Run, error) {
	if ctx == nil {
		return Run{}, domain.Invalid("simulation_context_required", "request context is required")
	}
	if err := scenario.Validate(); err != nil {
		return Run{}, domain.Invalid("simulation_scenario_invalid", "%v", err)
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Run{}, domain.Unavailable(codeRunUnavailable, "simulation service is shutting down")
	}
	if s.active {
		s.mu.Unlock()
		return Run{}, domain.Conflict(codeRunConflict, "a SUMO scenario is already running")
	}
	active, err := s.repo.ActiveRun(ctx)
	if err != nil {
		s.mu.Unlock()
		return Run{}, fmt.Errorf("check active simulation run: %w", err)
	}
	if active != nil {
		s.mu.Unlock()
		return Run{}, domain.Conflict(codeRunConflict, "a SUMO scenario is already running")
	}
	id, err := newRunID()
	if err != nil {
		s.mu.Unlock()
		return Run{}, err
	}
	run := Run{
		ID:        id,
		Status:    StatusQueued,
		Scenario:  scenario,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, run); err != nil {
		s.mu.Unlock()
		return Run{}, err
	}
	s.active = true
	s.wg.Add(1)
	s.mu.Unlock()

	go s.execute(run)
	return run, nil
}

func (s *Service) Get(ctx context.Context, id string) (Run, error) {
	if ctx == nil {
		return Run{}, domain.Invalid("simulation_context_required", "request context is required")
	}
	if id == "" {
		return Run{}, domain.Invalid("simulation_run_id_required", "run id is required")
	}
	run, err := s.repo.Get(ctx, id)
	if errors.Is(err, ErrRunNotFound) {
		return Run{}, domain.NotFound(codeRunNotFound, "simulation run %q does not exist", id)
	}
	if err != nil {
		return Run{}, fmt.Errorf("get simulation run: %w", err)
	}
	return run, nil
}

func (s *Service) List(ctx context.Context) ([]Run, error) {
	if ctx == nil {
		return nil, domain.Invalid("simulation_context_required", "request context is required")
	}
	runs, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list simulation runs: %w", err)
	}
	return runs, nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("shutdown context is required")
	}
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		_, _ = s.repo.RecoverInterrupted(context.WithoutCancel(s.ctx), "simulation cancelled by server shutdown")
		return fmt.Errorf("wait for SUMO scenario shutdown: %w", ctx.Err())
	}
}

func (s *Service) execute(run Run) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		s.active = false
		s.mu.Unlock()
	}()

	started := time.Now().UTC()
	run.Status = StatusRunning
	run.StartedAt = &started
	if err := s.repo.Update(s.ctx, run); err != nil {
		s.logger.Error("could not update SUMO run status", "run_id", run.ID, "error", err)
		finished := time.Now().UTC()
		run.Status = StatusFailed
		run.FinishedAt = &finished
		run.Error = fmt.Sprintf("could not start simulation run: %v", err)
		if updateErr := s.repo.Update(context.WithoutCancel(s.ctx), run); updateErr != nil {
			s.logger.Error("could not persist failed SUMO run", "run_id", run.ID, "error", updateErr)
		}
		return
	}

	result, err := s.executor.Run(s.ctx, run.Scenario)
	if err == nil {
		if deliveryErr := generateDeliveries(&result); deliveryErr != nil {
			err = fmt.Errorf("generate simulated delivery history: %w", deliveryErr)
		}
	}
	finished := time.Now().UTC()
	run.FinishedAt = &finished
	if err != nil {
		run.Status = StatusFailed
		if s.ctx.Err() != nil {
			run.Error = "simulation cancelled by server shutdown"
		} else {
			run.Error = err.Error()
		}
	} else {
		run.Status = StatusCompleted
		run.Result = &result
	}
	if updateErr := s.repo.Update(context.WithoutCancel(s.ctx), run); updateErr != nil {
		s.logger.Error("could not persist SUMO run result", "run_id", run.ID, "error", updateErr)
	}
}

func newRunID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", domain.Internal(codeRunIDFailed, "could not generate a simulation run id").WithCause(err)
	}
	return "sim_" + hex.EncodeToString(bytes[:]), nil
}

// ErrRunNotFound identifies a run that is absent from the repository.
var ErrRunNotFound = errors.New("simulation run not found")
