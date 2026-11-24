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
