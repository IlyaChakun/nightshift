-- +goose Up
-- A task queued with --allow-fallback may run on a fallback provider when the primary ones are at a long limit.
ALTER TABLE book.tasks ADD COLUMN allow_fallback boolean NOT NULL DEFAULT false;
