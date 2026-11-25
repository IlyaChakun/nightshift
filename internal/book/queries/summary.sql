-- name: CountTasksByState :many
SELECT state, count(*)::bigint AS tasks FROM book.tasks WHERE tenant = @tenant GROUP BY state;

-- CountFinishedSince counts tasks that reached an end state in the window.
-- name: CountFinishedSince :one
SELECT count(DISTINCT task_id)::bigint FROM book.events
WHERE tenant = @tenant AND kind = 'task.moved' AND created_at >= @since
  AND payload->>'to' IN ('released', 'merged_by_team', 'handed_off');

-- MergeStats are the two numbers of the summary: how many merged tasks never got a
-- "request changes", and the median time from creation to the merge.
-- name: MergeStats :one
WITH done AS (
    SELECT e.task_id, min(e.created_at) AS at FROM book.events e
    WHERE e.tenant = @tenant AND e.kind = 'task.moved' AND e.payload->>'to' IN ('released', 'merged_by_team')
    GROUP BY e.task_id
)
SELECT count(*)::bigint AS merged,
       count(*) FILTER (WHERE NOT EXISTS (
           SELECT 1 FROM book.events r
           WHERE r.task_id = done.task_id AND r.kind = 'task.moved' AND r.payload->>'event' = 'request_changes'
       ))::bigint AS without_changes,
       coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM done.at - t.created_at)), 0)::float8 AS median_seconds
FROM done JOIN book.tasks t ON t.id = done.task_id;

-- ProviderJobsSince counts the tenant's jobs that ended in the window, by provider and by
-- whether they hit a limit; the summary reports limit hits and the work done on fallback providers.
-- name: ProviderJobsSince :many
SELECT j.provider, (j.exit_reason = 'rate_limit')::boolean AS limited, count(*)::bigint AS jobs
FROM book.jobs j JOIN book.tasks t ON t.id = j.task_id
WHERE t.tenant = @tenant AND j.finished_at >= @since AND j.exit_reason IN ('done', 'rate_limit')
GROUP BY j.provider, limited
ORDER BY j.provider, limited;
