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

-- name: ListTasksWithStalePreview :many
-- Tasks in a state that needs no preview and that still have one on record: the conductor tears
-- each down.
SELECT t.* FROM book.tasks t
WHERE t.tenant = @tenant AND t.state = ANY(@states::text[])
  AND EXISTS (SELECT 1 FROM book.deploy_steps s WHERE s.task_id = t.id AND s.step = 'up')
ORDER BY t.id;

-- name: CountLivePreviews :one
-- Previews of the tenant that may still run, whatever their task's state (a teardown clears the
-- rows, so a preview counts until it is gone), and whether the asking task has one.
SELECT count(DISTINCT task_id) FILTER (WHERE task_id <> @task_id)::bigint AS others,
       coalesce(bool_or(task_id = @task_id), false)::boolean AS own
FROM book.deploy_steps
WHERE tenant = @tenant AND step = 'up';

-- name: ListTasksWithIdlePreview :many
-- Tasks that wait after acceptance (human review, handed off, ready, merging) and whose preview
-- was accepted before the cutoff: the conductor tears each down (design section 7, "Capacity").
-- The clock starts at the task's last move out of acceptance or into human_review, so a person
-- asked late (the train sends a task back for review) still gets the whole time; a task that
-- reached a waiting state another way counts from the time its preview came up.
SELECT t.* FROM book.tasks t
JOIN book.deploy_steps s ON s.task_id = t.id AND s.step = 'up'
WHERE t.tenant = @tenant AND t.state = ANY(@states::text[])
  AND coalesce(
        (SELECT max(e.created_at) FROM book.events e
         WHERE e.task_id = t.id AND e.kind = 'task.moved' AND (e.payload->>'from' = 'acceptance' OR e.payload->>'to' = 'human_review')),
        s.finished_at, s.started_at) < @accepted_before::timestamptz
ORDER BY t.id;
