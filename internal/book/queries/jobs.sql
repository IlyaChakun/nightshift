-- name: EnqueueJob :one
INSERT INTO book.jobs (tenant, task_id, role, provider, payload)
VALUES (@tenant, @task_id, @role, @provider, @payload)
RETURNING *;

-- LockClaims serializes claims inside their transaction: the caps below count running jobs,
-- and two claims that counted at the same instant could both take the last free place.
-- name: LockClaims :exec
SELECT pg_advisory_xact_lock(hashtext('book.jobs.claim'));

-- ClaimJob leases a queued job to a worker (design section 17, "Workers"). It takes the
-- oldest job of the tenant with the fewest running jobs per its weight (a tenant without a
-- weight weighs 1), so one tenant with a long queue cannot starve another, and it skips the
-- jobs of a provider that already runs as many jobs as its cap, so a provider's concurrency
-- holds across tenants however many loops enqueued. Both maps are JSON objects; a provider
-- without a positive cap has none; a weight below 0.01 counts as 0.01, so the division
-- cannot overflow. Run it after LockClaims and the expiry sweep, in the same transaction.
-- While the factory is paused (instance setting 'paused' is true) nothing is
-- leased; running jobs finish.
-- name: ClaimJob :one
UPDATE book.jobs
SET state = 'running', worker_id = @worker_id, token_sha256 = @token_sha256,
    lease_until = @lease_until, attempt = attempt + 1, started_at = coalesce(started_at, @now)
WHERE id = (
    SELECT j.id FROM book.jobs j
    WHERE j.state = 'queued'
      AND NOT EXISTS (SELECT 1 FROM book.settings WHERE tenant = '' AND key = 'paused' AND value = 'true'::jsonb)
      AND (coalesce((@provider_caps::jsonb ->> j.provider)::int, 0) <= 0
           OR (@provider_caps::jsonb ->> j.provider)::int
              > (SELECT count(*) FROM book.jobs r WHERE r.state = 'running' AND r.provider = j.provider))
    ORDER BY
      (SELECT count(*) FROM book.jobs r WHERE r.state = 'running' AND r.tenant = j.tenant)::float8
        / greatest(coalesce(nullif((@tenant_weights::jsonb ->> j.tenant)::float8, 0), 1), 0.01),
      j.id
    LIMIT 1 FOR UPDATE OF j SKIP LOCKED
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

-- AllTaskJobs lists every job of a task, of every role, for its timeline.
-- name: AllTaskJobs :many
SELECT * FROM book.jobs WHERE task_id = @task_id ORDER BY id;
