-- +goose Up
-- The journal of the merge train (design section 7): every step is written before it runs
-- and again when it is done, so a restart finds where the train stopped and reconciles with
-- GitHub instead of repeating a merge.
CREATE TABLE book.train_steps (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant      text        NOT NULL,
    task_id     bigint      NOT NULL REFERENCES book.tasks (id),
    step        text        NOT NULL,
    state       text        NOT NULL DEFAULT 'started',
    sha         text,
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    CONSTRAINT train_steps_once UNIQUE (task_id, step),
    CONSTRAINT train_steps_step CHECK (step IN ('rebase', 'ci', 'merge')),
    CONSTRAINT train_steps_state CHECK (state IN ('started', 'done')),
    CONSTRAINT train_steps_sha_hex CHECK (sha IS NULL OR sha ~ '^[0-9a-f]{40}$')
);

-- The journal is rewritten when a task goes back to checks, so the service may delete rows.
GRANT SELECT, INSERT, UPDATE, DELETE ON book.train_steps TO nightshift;
