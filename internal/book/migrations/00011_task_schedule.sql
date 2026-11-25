-- +goose Up
-- Plan ahead: a task does not start before not_before, and a higher priority goes first.
ALTER TABLE book.tasks ADD COLUMN not_before timestamptz;
ALTER TABLE book.tasks ADD COLUMN priority integer NOT NULL DEFAULT 0;
ALTER TABLE book.tasks ADD CONSTRAINT tasks_priority_range CHECK (priority BETWEEN -10 AND 10) NOT VALID; -- every old row is 0; new rows are checked
