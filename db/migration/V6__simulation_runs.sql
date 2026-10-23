create table simulation_runs (
    id            text primary key,
    status        text not null,
    scenario      jsonb not null,
    result        jsonb,
    error_message text not null default '',
    created_at    timestamptz not null,
    started_at    timestamptz,
    finished_at   timestamptz,

    constraint simulation_runs_status_known check (status in ('queued', 'running', 'completed', 'failed')),
    constraint simulation_runs_timing_ordered check (
        finished_at is null or started_at is null or finished_at >= started_at
    )
);

create index simulation_runs_created_at_idx on simulation_runs (created_at desc, id desc);
