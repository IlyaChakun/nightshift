-- +goose Up
-- The pull request a task is shipped in, set when nightshift opens it.
ALTER TABLE book.tasks ADD COLUMN pr_number integer;
