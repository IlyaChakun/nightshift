-- +goose Up
-- A task waits in `queued` until every task it depends on is released (design section 7).
-- Dependencies come from the approved spec (`to-tickets`).
CREATE TABLE book.task_deps (
    task_id    bigint NOT NULL REFERENCES book.tasks (id),
    depends_on bigint NOT NULL REFERENCES book.tasks (id),
    PRIMARY KEY (task_id, depends_on),
    CONSTRAINT task_deps_not_self CHECK (task_id <> depends_on)
);
CREATE INDEX task_deps_depends_on ON book.task_deps (depends_on);
GRANT SELECT, INSERT, DELETE ON book.task_deps TO nightshift;
