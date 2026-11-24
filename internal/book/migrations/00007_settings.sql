-- +goose Up
-- Operational switches (design section 15): review mode, SLA auto-merge, pause, parallelism.
-- Static configuration lives in git; these change from the panel or a /ns command and every
-- change is also an event with its author. Tenant '' holds the settings of the whole instance.
CREATE TABLE book.settings (
    tenant     text        NOT NULL,
    key        text        NOT NULL,
    value      jsonb       NOT NULL,
    updated_by text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, key)
);
GRANT SELECT, INSERT, UPDATE ON book.settings TO nightshift;
