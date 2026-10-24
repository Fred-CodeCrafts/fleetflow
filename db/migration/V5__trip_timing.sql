-- Trip timing.
--
-- PRD US-9 asks a completed trip to be listed with its start, end and duration,
-- and created_at alone cannot answer that: it says when the row was written, not
-- when the vehicle left or arrived.
--
-- Both columns are nullable on purpose. A trip that has not started has no start
-- time, and a trip that jumped straight from planned to completed never recorded
-- one. Backfilling those with created_at would invent a duration, and a made-up
-- number in a performance table is worse than a blank cell. The API reports null
-- and the console shows "not recorded".
--
-- The check constraint is the same rule the domain enforces, restated where the
-- data lives: a finish cannot precede a start. It is written to pass when either
-- side is null, because "unknown" is not a violation.
alter table trips
    add column started_at  timestamptz,
    add column finished_at timestamptz;

alter table trips
    add constraint trips_timing_ordered
    check (finished_at is null or started_at is null or finished_at >= started_at);

-- Completed trips are listed newest first on the history screen, which filters
-- on status. Without this the query scans the whole table.
create index trips_completed_finished_idx
    on trips (finished_at desc)
    where status = 'completed';
