-- +goose Up
-- An event a job's consumption writes names that job, so a consumption that is repeated after a
-- failure writes it once. A plain index build: book.events holds one instance's history, so
-- the lock on inserts is short.
ALTER TABLE book.events ADD COLUMN job_id bigint;
CREATE UNIQUE INDEX events_job_kind ON book.events (job_id, kind) WHERE job_id IS NOT NULL;
