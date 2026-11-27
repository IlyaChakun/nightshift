-- name: AppendEvent :one
INSERT INTO book.events (tenant, task_id, kind, actor, payload, trace_id)
VALUES (@tenant, @task_id, @kind, @actor, @payload, @trace_id)
RETURNING id;

-- name: ListTaskEvents :many
SELECT * FROM book.events WHERE task_id = @task_id ORDER BY id;

-- RecordDecision returns no row when the same decision was already recorded (idempotent).
-- name: RecordDecision :one
INSERT INTO book.decisions (task_id, kind, sha, actor, card_version, reason)
VALUES (@task_id, @kind, @sha, @actor, @card_version, @reason)
ON CONFLICT ON CONSTRAINT decisions_once DO NOTHING
RETURNING id;

-- name: ListDecisions :many
SELECT * FROM book.decisions WHERE task_id = @task_id ORDER BY id;

-- AppendJobEvent records an event of a job's consumption once: a repeat with the same job and
-- kind writes nothing and returns 0 rows.
-- name: AppendJobEvent :execrows
INSERT INTO book.events (tenant, task_id, kind, actor, payload, job_id)
VALUES (@tenant, @task_id, @kind, @actor, @payload, @job_id)
ON CONFLICT (job_id, kind) WHERE job_id IS NOT NULL DO NOTHING;
