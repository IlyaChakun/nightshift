-- name: CreateTask :one
INSERT INTO book.tasks (tenant, repo, title, thread_ref)
VALUES (@tenant, @repo, @title, @thread_ref)
RETURNING *;

-- name: GetTask :one
SELECT * FROM book.tasks WHERE id = @id;

-- ListActiveTasks is what the conductor looks at each tick.
-- name: ListActiveTasks :many
SELECT * FROM book.tasks WHERE tenant = @tenant AND state = ANY(@states::text[]) ORDER BY priority DESC, id;

-- name: ListTasksByState :many
SELECT * FROM book.tasks WHERE tenant = @tenant AND state = @state ORDER BY id;

-- TransitionTask is the compare-and-set of design section 7: it changes the task only if
-- state, version and head_sha are still what the caller read. No row means a lost race.
-- name: TransitionTask :one
UPDATE book.tasks
SET state = @to_state, version = version + 1, head_sha = @new_head_sha, returns = @new_returns, updated_at = now()
WHERE id = @id
  AND state = @from_state
  AND version = @version
  AND head_sha IS NOT DISTINCT FROM @head_sha
RETURNING *;

-- name: CreateBudget :exec
INSERT INTO book.budgets (task_id, max_iterations, max_tokens, max_duration)
VALUES (@task_id, @max_iterations, @max_tokens, @max_duration);

-- name: GetBudget :one
SELECT * FROM book.budgets WHERE task_id = @task_id;

-- name: SpendBudget :one
UPDATE book.budgets
SET used_iterations = used_iterations + @iterations, used_tokens = used_tokens + @tokens
WHERE task_id = @task_id
RETURNING *;

-- GetTaskByThread finds the task a chat thread belongs to.
-- name: GetTaskByThread :one
SELECT * FROM book.tasks WHERE tenant = @tenant AND thread_ref = @thread_ref ORDER BY id LIMIT 1;

-- name: SetTaskPR :exec
UPDATE book.tasks SET pr_number = @pr_number WHERE id = @id;

-- CreateQueuedTask adds a task the owner puts straight into the queue (`nightshift task add`),
-- without a chat thread or a spec.
-- name: CreateQueuedTask :one
INSERT INTO book.tasks (tenant, repo, title, state, not_before, priority)
VALUES (@tenant, @repo, @title, 'queued', @not_before, @priority)
RETURNING *;
