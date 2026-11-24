-- +goose Up
-- How many times review sent the task back to Builder; the state machine escalates at the limit.
ALTER TABLE book.tasks ADD COLUMN returns integer NOT NULL DEFAULT 0;
