-- +goose Up
ALTER TABLE book.findings ADD COLUMN ci_checks text[];
