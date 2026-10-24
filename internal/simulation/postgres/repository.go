package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fleetflow/internal/domain"
	"fleetflow/internal/simulation"
	"fleetflow/internal/simulation/sumo"
)

const (
	uniqueViolation = "23505"
	codeRunExists   = "simulation_run_exists"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, run simulation.Run) error {
	scenario, err := json.Marshal(run.Scenario)
	if err != nil {
		return fmt.Errorf("encode simulation scenario: %w", err)
	}
	const query = `insert into simulation_runs
		(id, status, scenario, error_message, created_at, started_at, finished_at)
		values ($1, $2, $3, $4, $5, $6, $7)`
	if _, err := r.pool.Exec(ctx, query, run.ID, run.Status, scenario, run.Error, run.CreatedAt, run.StartedAt, run.FinishedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			if pgErr.ConstraintName == "simulation_runs_active_unique" {
				return domain.Conflict(simulation.CodeRunConflict, "a SUMO scenario is already running").WithCause(err)
			}
			return domain.Conflict(codeRunExists, "simulation run %q already exists", run.ID).WithCause(err)
		}
		return err
	}
	return nil
}

func (r *Repository) ActiveRun(ctx context.Context) (*simulation.Run, error) {
	const query = `select id, status, scenario, error_message, created_at, started_at, finished_at
		from simulation_runs
		where status in ('queued', 'running')
		order by created_at desc
		limit 1`
	run, err := scanRun(r.pool.QueryRow(ctx, query), false)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *Repository) RecoverInterrupted(ctx context.Context, reason string) ([]simulation.Run, error) {
	const selectQuery = `select id, status, scenario, error_message, created_at, started_at, finished_at
		from simulation_runs
		where status in ('queued', 'running')`
	rows, err := r.pool.Query(ctx, selectQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var toRecover []simulation.Run
	for rows.Next() {
		run, err := scanRun(rows, false)
		if err != nil {
			return nil, err
		}
		toRecover = append(toRecover, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(toRecover) == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	const updateQuery = `update simulation_runs
		set status = 'failed',
		    error_message = $2,
		    finished_at = $3
		where id = $1 and status in ('queued', 'running')`
	var recovered []simulation.Run
	for _, run := range toRecover {
		errMsg := reason
		if run.Error != "" {
			errMsg = run.Error + "; " + reason
		}
		tag, err := r.pool.Exec(ctx, updateQuery, run.ID, errMsg, now)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() > 0 {
			run.Status = simulation.StatusFailed
			run.Error = errMsg
			run.FinishedAt = &now
			recovered = append(recovered, run)
		}
	}
	return recovered, nil
}

func (r *Repository) Update(ctx context.Context, run simulation.Run) error {
	scenario, err := json.Marshal(run.Scenario)
	if err != nil {
		return fmt.Errorf("encode simulation scenario: %w", err)
	}
	var result []byte
	if run.Result != nil {
		result, err = json.Marshal(run.Result)
		if err != nil {
			return fmt.Errorf("encode simulation result: %w", err)
		}
	}
	const query = `update simulation_runs
		set status = $2, scenario = $3, result = $4, error_message = $5,
			started_at = $6, finished_at = $7
		where id = $1`
	tag, err := r.pool.Exec(ctx, query, run.ID, run.Status, scenario, result, run.Error, run.StartedAt, run.FinishedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return simulation.ErrRunNotFound
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (simulation.Run, error) {
	const query = `select id, status, scenario, result, error_message, created_at, started_at, finished_at
		from simulation_runs where id = $1`
	run, err := scanRun(r.pool.QueryRow(ctx, query, id), true)
	if errors.Is(err, pgx.ErrNoRows) {
		return simulation.Run{}, simulation.ErrRunNotFound
	}
	return run, err
}

func (r *Repository) List(ctx context.Context) ([]simulation.Run, error) {
	const query = `select id, status, scenario, error_message, created_at, started_at, finished_at
		from simulation_runs order by created_at desc, id desc limit 100`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := make([]simulation.Run, 0)
	for rows.Next() {
		run, err := scanRun(rows, false)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanRun(row rowScanner, includeResult bool) (simulation.Run, error) {
	var run simulation.Run
	var scenario []byte
	var result []byte
	if includeResult {
		err := row.Scan(&run.ID, &run.Status, &scenario, &result, &run.Error, &run.CreatedAt, &run.StartedAt, &run.FinishedAt)
		if err != nil {
			return simulation.Run{}, err
		}
	} else if err := row.Scan(&run.ID, &run.Status, &scenario, &run.Error, &run.CreatedAt, &run.StartedAt, &run.FinishedAt); err != nil {
		return simulation.Run{}, err
	}
	if err := json.Unmarshal(scenario, &run.Scenario); err != nil {
		return simulation.Run{}, fmt.Errorf("decode stored simulation scenario: %w", err)
	}
	if len(result) > 0 && string(result) != "null" {
		var decoded sumo.Result
		if err := json.Unmarshal(result, &decoded); err != nil {
			return simulation.Run{}, fmt.Errorf("decode stored simulation result: %w", err)
		}
		run.Result = &decoded
	}
	return run, nil
}
