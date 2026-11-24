-- +goose Up
-- The book: source of truth for tasks (design sections 3, 7 and 14).
-- Migrations are expand-only: never drop or rename; add new columns and tables instead.
-- They run as the schema owner (deploy/upgrade.sh), which needs CREATEROLE.

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'nightshift') THEN
        CREATE ROLE nightshift NOLOGIN;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'grafana_ro') THEN
        CREATE ROLE grafana_ro NOLOGIN;
    END IF;
END
$$;
-- +goose StatementEnd

CREATE SCHEMA book;
REVOKE ALL ON SCHEMA book FROM PUBLIC;
GRANT USAGE ON SCHEMA book TO nightshift;

CREATE TABLE book.tasks (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant      text        NOT NULL,
    repo        text        NOT NULL, -- groundwork for multi-repository tasks (phase 10)
    title       text        NOT NULL,
    state       text        NOT NULL DEFAULT 'idea',
    version     integer     NOT NULL DEFAULT 1, -- CAS on (state, version, head_sha)
    head_sha    text,
    thread_ref  text,       -- chat thread of the task
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tasks_version_positive CHECK (version > 0),
    CONSTRAINT tasks_head_sha_hex CHECK (head_sha IS NULL OR head_sha ~ '^[0-9a-f]{40}$')
);
CREATE INDEX tasks_tenant_state ON book.tasks (tenant, state);

-- Append-only: the trigger rejects changes even from the table owner.
CREATE TABLE book.events (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant      text        NOT NULL,
    task_id     bigint      REFERENCES book.tasks (id),
    kind        text        NOT NULL,
    actor       text        NOT NULL, -- 'system', a role, or the actor's chat/GitHub/tracker id
    payload     jsonb       NOT NULL DEFAULT '{}',
    trace_id    text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX events_task ON book.events (task_id, id);

-- +goose StatementBegin
CREATE FUNCTION book.events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'book.events is append-only: % is not allowed', TG_OP;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER events_no_update_delete BEFORE UPDATE OR DELETE ON book.events
    FOR EACH ROW EXECUTE FUNCTION book.events_append_only();
CREATE TRIGGER events_no_truncate BEFORE TRUNCATE ON book.events
    FOR EACH STATEMENT EXECUTE FUNCTION book.events_append_only();

-- Decisions are bound to a SHA; repeating the same decision is idempotent.
CREATE TABLE book.decisions (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    task_id      bigint      NOT NULL REFERENCES book.tasks (id),
    kind         text        NOT NULL,
    sha          text        NOT NULL DEFAULT '',
    actor        text        NOT NULL,
    card_version integer,
    reason       text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT decisions_kind CHECK (kind IN
        ('approve', 'review', 'edit', 'reject', 'request_changes', 'merge', 'rollback', 'cancel')),
    CONSTRAINT decisions_once UNIQUE (task_id, kind, sha, actor)
);

-- One budget per task: iterations, tokens, hours (design section 7).
CREATE TABLE book.budgets (
    task_id         bigint      PRIMARY KEY REFERENCES book.tasks (id),
    max_iterations  integer     NOT NULL,
    max_tokens      bigint      NOT NULL,
    max_duration    interval    NOT NULL,
    used_iterations integer     NOT NULL DEFAULT 0,
    used_tokens     bigint      NOT NULL DEFAULT 0,
    started_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT budgets_limits_positive CHECK (max_iterations > 0 AND max_tokens > 0 AND max_duration > interval '0'),
    CONSTRAINT budgets_used_nonnegative CHECK (used_iterations >= 0 AND used_tokens >= 0)
);

-- Every side effect towards chat or tracker goes through the outbox, secrets masked first.
CREATE TABLE book.outbox (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant          text        NOT NULL,
    task_id         bigint      REFERENCES book.tasks (id),
    target          text        NOT NULL, -- 'chat', 'tracker', ...
    payload         jsonb       NOT NULL,
    idempotency_key text        NOT NULL UNIQUE,
    status          text        NOT NULL DEFAULT 'pending',
    attempts        integer     NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz,
    CONSTRAINT outbox_status CHECK (status IN ('pending', 'sent', 'failed'))
);
CREATE INDEX outbox_due ON book.outbox (next_attempt_at) WHERE status = 'pending';

-- The service role: no UPDATE or DELETE on events.
GRANT SELECT, INSERT, UPDATE ON book.tasks, book.budgets, book.outbox TO nightshift;
GRANT SELECT, INSERT ON book.events, book.decisions TO nightshift;

-- Dashboards read only views, never the tables (design section 15).
CREATE VIEW book.dash_task_states AS
    SELECT tenant, state, count(*) AS tasks FROM book.tasks GROUP BY tenant, state;
GRANT USAGE ON SCHEMA book TO grafana_ro;
GRANT SELECT ON book.dash_task_states TO grafana_ro;
GRANT SELECT ON book.dash_task_states TO nightshift;
