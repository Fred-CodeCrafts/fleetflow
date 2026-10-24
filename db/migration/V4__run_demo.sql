-- A saved run records whether it came from a demo deployment (TASKS T40a).
--
-- A run takes the engine's weights: it clears them at start and then feeds them.
-- That is only correct when nothing live is reporting, which is what `-demo`
-- asserts, so the flag is a property of the run rather than of the request. Kept as
-- a column instead of a field inside the config document because the config is what
-- the caller asked for, and this is what the deployment was.
--
-- No default and not null: every run this schema accepts either was a demo run or
-- was not, and a row that does not say would be a record that cannot be read back
-- honestly. Existing rows predate the column, so they are backfilled as false,
-- which is what they were: nothing could start a run without a demo flag before it
-- existed only in the sense that there was no flag to have.
alter table runs add column demo boolean not null default false;

-- The default is for the backfill only. Dropping it means a future insert has to
-- state the answer instead of inheriting one.
alter table runs alter column demo drop default;
