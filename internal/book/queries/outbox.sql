-- EnqueueOutbox returns no row when the idempotency key is already queued.
-- name: EnqueueOutbox :one
INSERT INTO book.outbox (tenant, task_id, target, payload, idempotency_key)
VALUES (@tenant, @task_id, @target, @payload, @idempotency_key)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING id;

-- ClaimOutbox locks due messages; concurrent workers skip each other's rows.
-- name: ClaimOutbox :many
SELECT * FROM book.outbox
WHERE status = 'pending' AND next_attempt_at <= now()
ORDER BY id
LIMIT @max_rows
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxSent :exec
UPDATE book.outbox SET status = 'sent', sent_at = now(), attempts = attempts + 1 WHERE id = @id;

-- name: MarkOutboxRetry :exec
UPDATE book.outbox
SET attempts = attempts + 1, last_error = @last_error, next_attempt_at = @next_attempt_at
WHERE id = @id;

-- name: MarkOutboxFailed :exec
UPDATE book.outbox SET status = 'failed', attempts = attempts + 1, last_error = @last_error WHERE id = @id;

-- ParkOutbox holds a message until a later time without counting an attempt: quiet hours, not a failure.
-- name: ParkOutbox :exec
UPDATE book.outbox SET next_attempt_at = @next_attempt_at WHERE id = @id AND status = 'pending';
