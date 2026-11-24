-- +goose Up
-- Findings of every layer (design sections 8 and 10): the raw material for metrics and learn.
-- The same fingerprint can appear in several jobs of a task, so it is not unique.
CREATE TABLE book.findings (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant         text        NOT NULL,
    task_id        bigint      NOT NULL REFERENCES book.tasks (id),
    job_id         bigint      REFERENCES book.jobs (id),
    rule_id        text        NOT NULL,
    category       text        NOT NULL,
    severity       text        NOT NULL,
    layer_caught   text        NOT NULL,
    layer_expected text,
    message        text        NOT NULL,
    path           text,
    start_line     integer,
    end_line       integer,
    fingerprint    text        NOT NULL DEFAULT '',
    status         text        NOT NULL DEFAULT 'open',
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT findings_severity CHECK (severity IN ('block', 'warn')),
    CONSTRAINT findings_status CHECK (status IN ('open', 'fixed', 'disputed', 'false_positive'))
);
CREATE INDEX findings_task ON book.findings (task_id, id);
CREATE INDEX findings_rule ON book.findings (tenant, rule_id);

-- What agents want remembered. An agent never writes to the book itself: nightshift stores
-- each note from a validated role result as proposed, with provenance. Notes are never put
-- into a prompt automatically (design section 19).
CREATE TABLE book.notes (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant      text        NOT NULL,
    task_id     bigint      NOT NULL REFERENCES book.tasks (id),
    job_id      bigint      NOT NULL REFERENCES book.jobs (id),
    role        text        NOT NULL,
    kind        text        NOT NULL,
    body        text        NOT NULL,
    source      text        NOT NULL,
    status      text        NOT NULL DEFAULT 'proposed',
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT notes_status CHECK (status IN ('proposed', 'promoted', 'rejected'))
);
CREATE INDEX notes_tenant_status ON book.notes (tenant, status, id);

GRANT SELECT, INSERT, UPDATE ON book.findings, book.notes TO nightshift;
