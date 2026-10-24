-- Saved comparison runs (DESIGN 3.5, 3.7; PRD US-9).
--
-- A run is one finished comparison between the baseline and the dynamic-weight
-- arm: the demand it was started with (seed and config) and the metrics it ended
-- with, kept whole rather than shredded into columns. Storing the two documents
-- as the API serialized them is deliberate: the API already owns their shape, so
-- the database cannot disagree with it, and a field added to a metric later needs
-- no migration to be recorded.
--
-- The seed is a plain bigint, zero and negative values included. What a seed
-- means belongs to the simulator, which is the only layer that defines it, so the
-- store must not judge the number it is handed.

create table runs (
    id         text primary key,
    seed       bigint not null,
    config     jsonb not null,
    metrics    jsonb not null,
    created_at timestamptz not null default now()
);

-- The run list is read most recent first, which is the order a dashboard shows
-- saved runs in, so the index carries that direction and the read never sorts.
-- created_at alone is not unique: two runs saved in the same instant share a
-- timestamp, so id is the tiebreaker, exactly as trips order by created_at then
-- id. The index is not partial or covering because every column the list needs is
-- small and the row is read anyway.
create index runs_created_at_idx on runs (created_at desc, id desc);
