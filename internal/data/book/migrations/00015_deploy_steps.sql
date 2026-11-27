-- +goose Up
-- The journal of deploy steps (design section 17, "Deploy targets"): every step of a preview or a
-- release is written before it runs and again when it is done, so after a restart the conductor
-- continues from the row instead of repeating a release, and a rollback still knows which revision
-- was live before.
CREATE TABLE book.deploy_steps (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant      text        NOT NULL,
    task_id     bigint      NOT NULL REFERENCES book.tasks (id),
    step        text        NOT NULL,
    state       text        NOT NULL DEFAULT 'started',
    revision    text,
    previous    text,
    ok          boolean,
    detail      text        NOT NULL DEFAULT '',
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    CONSTRAINT deploy_steps_once UNIQUE (task_id, step),
    CONSTRAINT deploy_steps_step CHECK (step IN ('up', 'smoke', 'release', 'release_smoke', 'rollback')),
    CONSTRAINT deploy_steps_state CHECK (state IN ('started', 'done')),
    CONSTRAINT deploy_steps_revision_hex CHECK (revision IS NULL OR revision ~ '^[0-9a-f]{40}$'),
    CONSTRAINT deploy_steps_previous_hex CHECK (previous IS NULL OR previous ~ '^[0-9a-f]{40}$')
);

-- A task that goes back to Builder starts a new preview, so the service may delete rows.
GRANT SELECT, INSERT, UPDATE, DELETE ON book.deploy_steps TO nightshift;
