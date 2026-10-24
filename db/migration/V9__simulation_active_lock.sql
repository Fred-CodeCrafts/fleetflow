-- Enforce that at most one simulation run can be queued or running at any
-- time, providing a durable lock across multiple Cloud Run instances.
create unique index simulation_runs_active_unique on simulation_runs ((1))
    where status in ('queued', 'running');
