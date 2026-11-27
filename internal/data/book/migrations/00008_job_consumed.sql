-- +goose Up
-- The conductor marks a finished job once it has acted on the result, so a restart never
-- applies one patch twice.
ALTER TABLE book.jobs ADD COLUMN consumed_at timestamptz;
CREATE INDEX jobs_unconsumed ON book.jobs (task_id, id) WHERE consumed_at IS NULL;
