-- Fleet records: vehicles, drivers and trips (DESIGN 3.7).
--
-- Statuses are stored as their wire names, the same strings the API accepts, so
-- the two cannot drift apart: a token the handler rejects is a token the
-- database also refuses.
--
-- The rules that must survive concurrency are constraints here rather than
-- checks in Go. Two requests racing to put the same vehicle on a trip are
-- stopped by the database, whatever the service did before them.

create table vehicles (
    id         text primary key,
    name       text not null,
    type       text not null default '',
    status     text not null,
    created_at timestamptz not null default now(),

    constraint vehicles_status_known check (status in ('available', 'on_trip', 'offline'))
);

-- The dispatcher filters vehicles by availability.
create index vehicles_status_idx on vehicles (status);

create table drivers (
    id         text primary key,
    name       text not null,
    contact    text not null default '',
    vehicle_id text references vehicles (id),
    created_at timestamptz not null default now(),

    -- A driver holds at most one vehicle_id, which is already "one vehicle per
    -- driver". The unique constraint adds the other direction: one vehicle
    -- cannot be claimed by two drivers. Postgres treats nulls as distinct here,
    -- so any number of drivers may be unassigned.
    constraint drivers_one_per_vehicle unique (vehicle_id)
);

create table trips (
    id          text primary key,
    origin      text not null,
    destination text not null,
    vehicle_id  text references vehicles (id),
    status      text not null,
    created_at  timestamptz not null default now(),

    constraint trips_status_known check (status in ('planned', 'in_progress', 'completed'))
);

-- A vehicle may be on only one unfinished trip. This index is what turns a
-- concurrent double assignment into a failed insert instead of two active trips.
create unique index trips_one_active_per_vehicle
    on trips (vehicle_id)
    where vehicle_id is not null and status <> 'completed';

-- The trip list filters by vehicle and by status.
create index trips_vehicle_idx on trips (vehicle_id);
create index trips_status_idx on trips (status);
