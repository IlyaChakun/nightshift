-- name: EnqueueJob :one
INSERT INTO book.jobs (tenant, task_id, role, provider, payload)
VALUES (@tenant, @task_id, @role, @provider, @payload)
RETURNING *;

-- ClaimJob leases the oldest queued job to a worker. SKIP LOCKED lets parallel claims
-- take different jobs without waiting on each other. While the factory is paused
-- (instance setting 'paused' is true) nothing is leased; running jobs finish.
-- name: ClaimJob :one
UPDATE book.jobs
SET state = 'running', worker_id = @worker_id, token_sha256 = @token_sha256,
    lease_until = @lease_until, attempt = attempt + 1, started_at = coalesce(started_at, @now)
WHERE id = (
    SELECT id FROM book.jobs
    WHERE state = 'queued'
      AND NOT EXISTS (SELECT 1 FROM book.settings WHERE tenant = '' AND key = 'paused' AND value = 'true'::jsonb)
    ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- GetLeasedJob finds a running job by its token; an expired lease does not match.
-- name: GetLeasedJob :one
SELECT * FROM book.jobs
WHERE id = @id AND state = 'running' AND token_sha256 = @token_sha256 AND lease_until > @now;

-- name: ExtendLease :one
UPDATE book.jobs SET lease_until = @lease_until
WHERE id = @id AND state = 'running' AND token_sha256 = @token_sha256 AND lease_until > @now
RETURNING lease_until;

-- CompleteJob ends a lease once; the token is cleared, so a repeat finds no row.
-- name: CompleteJob :one
UPDATE book.jobs
SET state = @state, exit_reason = @exit_reason, result = @result, usage = @usage,
    token_sha256 = NULL, lease_until = NULL, finished_at = @now
WHERE id = @id AND state = 'running' AND token_sha256 = @token_sha256 AND lease_until > @now
RETURNING *;

-- RequeueExpiredJobs returns jobs whose worker vanished to the queue while attempts remain.
-- name: RequeueExpiredJobs :many
UPDATE book.jobs
SET state = 'queued', worker_id = NULL, token_sha256 = NULL, lease_until = NULL
WHERE state = 'running' AND lease_until <= @now AND attempt < @max_attempts
RETURNING id;

-- name: FailExpiredJobs :many
UPDATE book.jobs
SET state = 'failed', exit_reason = 'lease_expired', token_sha256 = NULL, lease_until = NULL, finished_at = @now
WHERE state = 'running' AND lease_until <= @now AND attempt >= @max_attempts
RETURNING id;

-- name: AddArtifact :exec
INSERT INTO book.artifacts (job_id, path, sha256, bytes)
VALUES (@job_id, @path, @sha256, @bytes)
ON CONFLICT ON CONSTRAINT artifacts_once DO NOTHING;

-- name: ListJobArtifacts :many
SELECT * FROM book.artifacts WHERE job_id = @job_id ORDER BY path;

-- name: SaveHarnessSession :exec
INSERT INTO book.harness_sessions (task_id, role, provider, session_id)
VALUES (@task_id, @role, @provider, @session_id)
ON CONFLICT (task_id, role) DO UPDATE
SET provider = excluded.provider, session_id = excluded.session_id, updated_at = now();

-- name: GetHarnessSession :one
SELECT * FROM book.harness_sessions WHERE task_id = @task_id AND role = @role;

-- CancelTaskJobs stops a canceled task's work: queued jobs never start and a running job
-- loses its token, so its next heartbeat or result is refused.
-- name: CancelTaskJobs :execrows
UPDATE book.jobs
SET state = 'canceled', token_sha256 = NULL, lease_until = NULL
WHERE task_id = @task_id AND state IN ('queued', 'running');

-- name: ListTaskJobs :many
SELECT * FROM book.jobs WHERE task_id = @task_id AND role = @role ORDER BY id;

-- ConsumeJob marks a finished job as acted on; no row means another run already did it.
-- name: ConsumeJob :one
UPDATE book.jobs SET consumed_at = now()
WHERE id = @id AND consumed_at IS NULL AND state IN ('done', 'failed', 'canceled')
RETURNING id;

-- name: ListJobFindings :many
SELECT * FROM book.findings WHERE job_id = @job_id ORDER BY id;

-- name: CountTaskEvents :one
SELECT count(*)::bigint FROM book.events WHERE task_id = @task_id AND kind = @kind;
