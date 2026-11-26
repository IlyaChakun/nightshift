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
