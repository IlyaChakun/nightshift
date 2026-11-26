-- name: StartDeployStep :one
-- Starting a step again after a restart keeps a finished step as it is.
INSERT INTO book.deploy_steps (tenant, task_id, step)
VALUES (@tenant, @task_id, @step)
ON CONFLICT (task_id, step) DO UPDATE SET step = excluded.step
RETURNING *;

-- name: FinishDeployStep :exec
UPDATE book.deploy_steps
SET state = 'done', revision = @revision, previous = @previous, ok = @ok, detail = @detail, finished_at = now()
WHERE task_id = @task_id AND step = @step;

-- name: ClearDeploySteps :exec
-- A new preview of the same task starts from nothing; the release steps stay for the audit.
DELETE FROM book.deploy_steps WHERE task_id = @task_id AND step IN ('up', 'smoke');

-- name: ResetDeployStep :exec
-- A step that belonged to another revision runs again.
DELETE FROM book.deploy_steps WHERE task_id = @task_id AND step = @step;

-- name: ListEndedTasksWithPreview :many
-- Tasks that left the flow and still have a preview on record: the conductor tears each down.
SELECT t.* FROM book.tasks t
WHERE t.tenant = @tenant AND t.state = ANY(@states::text[])
  AND EXISTS (SELECT 1 FROM book.deploy_steps s WHERE s.task_id = t.id AND s.step = 'up')
ORDER BY t.id;

-- name: CountLivePreviews :one
-- Previews of the tenant that may still run (a preview row of a task that has not ended; an
-- ended task's preview goes at the next teardown sweep), and whether the asking task has one.
SELECT count(DISTINCT s.task_id) FILTER (WHERE s.task_id <> @task_id)::bigint AS others,
       coalesce(bool_or(s.task_id = @task_id), false)::boolean AS own
FROM book.deploy_steps s JOIN book.tasks t ON t.id = s.task_id
WHERE s.tenant = @tenant AND s.step = 'up' AND NOT (t.state = ANY(@ended::text[]));
