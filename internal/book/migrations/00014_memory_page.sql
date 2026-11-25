-- +goose Up
-- The project memory page: one per tenant, small and capped (design section 13).
-- `revision` is the compare-and-swap token: a write names the revision it read.
CREATE TABLE book.memory_pages (
    tenant     text        PRIMARY KEY,
    revision   bigint      NOT NULL,
    entries    jsonb       NOT NULL DEFAULT '[]',
    updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT, UPDATE ON book.memory_pages TO nightshift;
