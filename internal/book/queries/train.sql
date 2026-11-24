-- name: StartTrainStep :one
-- Starting a step again after a restart keeps a finished step as it is.
INSERT INTO book.train_steps (tenant, task_id, step)
VALUES (@tenant, @task_id, @step)
ON CONFLICT (task_id, step) DO UPDATE SET step = excluded.step
RETURNING *;

-- name: FinishTrainStep :exec
UPDATE book.train_steps SET state = 'done', sha = @sha, finished_at = now()
WHERE task_id = @task_id AND step = @step;

-- name: ListTrainSteps :many
SELECT * FROM book.train_steps WHERE task_id = @task_id ORDER BY id;

-- name: ClearTrainSteps :exec
-- A task that goes back to Builder starts a new train run.
DELETE FROM book.train_steps WHERE task_id = @task_id;
