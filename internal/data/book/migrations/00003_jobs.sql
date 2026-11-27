-- +goose Up
-- Jobs for the worker, their leases and tokens, harness sessions and artifacts
-- (design sections 3 and 12, phase 2). The worker never reads these tables; nightshift
-- serves them over the jobs API.

CREATE TABLE book.jobs (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant        text        NOT NULL,
    task_id       bigint      NOT NULL REFERENCES book.tasks (id),
    role          text        NOT NULL,
    provider      text        NOT NULL,
    state         text        NOT NULL DEFAULT 'queued',
    attempt       integer     NOT NULL DEFAULT 0,
    payload       jsonb       NOT NULL, -- the role's view of the task (jobsapi.Job)
    worker_id     text,
    token_sha256  text,       -- hash of the job token of the current lease; never the token
    lease_until   timestamptz,
    exit_reason   text,
    result        jsonb,
    usage         jsonb,
    created_at    timestamptz NOT NULL DEFAULT now(),
    started_at    timestamptz,
    finished_at   timestamptz,
    CONSTRAINT jobs_state CHECK (state IN ('queued', 'running', 'done', 'failed', 'canceled')),
    CONSTRAINT jobs_running_has_lease CHECK (state <> 'running' OR (token_sha256 IS NOT NULL AND lease_until IS NOT NULL)),
    CONSTRAINT jobs_token_hex CHECK (token_sha256 IS NULL OR token_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT jobs_exit_reason CHECK (exit_reason IS NULL OR exit_reason IN ('done', 'budget', 'rate_limit', 'error', 'lease_expired'))
);
CREATE INDEX jobs_queued ON book.jobs (id) WHERE state = 'queued';
CREATE INDEX jobs_running_lease ON book.jobs (lease_until) WHERE state = 'running';
CREATE INDEX jobs_task ON book.jobs (task_id, id);

-- One harness session per (task, role): the next turn continues it (design section 12).
CREATE TABLE book.harness_sessions (
    task_id     bigint      NOT NULL REFERENCES book.tasks (id),
    role        text        NOT NULL,
    provider    text        NOT NULL,
    session_id  text        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, role)
);

-- Files a job wrote; the hash makes tampering after completion visible.
CREATE TABLE book.artifacts (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id      bigint      NOT NULL REFERENCES book.jobs (id),
    path        text        NOT NULL,
    sha256      text        NOT NULL,
    bytes       bigint      NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT artifacts_once UNIQUE (job_id, path),
    CONSTRAINT artifacts_sha256_hex CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT artifacts_bytes_nonnegative CHECK (bytes >= 0)
);

GRANT SELECT, INSERT, UPDATE ON book.jobs, book.harness_sessions TO nightshift;
GRANT SELECT, INSERT ON book.artifacts TO nightshift;
