-- +goose Up
-- An event a job's consumption writes names that job, so a consumption that is repeated after a
-- failure writes it once.
ALTER TABLE book.events ADD COLUMN job_id bigint;
CREATE UNIQUE INDEX events_job_kind ON book.events (job_id, kind) WHERE job_id IS NOT NULL;
