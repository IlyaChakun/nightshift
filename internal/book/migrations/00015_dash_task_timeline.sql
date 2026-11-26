-- +goose Up
-- One line per step of a task, for the Grafana task timeline (design section 15).
-- The same lines as `nightshift status --task`, except for payloads and harness errors: an event
-- shows its kind (a move shows from, to and event), and a job shows only the supervisor's
-- `stopped` reason, because grafana_ro reads no free text from agents or harnesses.
CREATE VIEW book.dash_task_timeline AS
    SELECT e.tenant, e.task_id, e.created_at AS at, e.actor AS who,
           CASE WHEN e.kind = 'task.moved'
                THEN concat_ws(' ', e.payload ->> 'from', '->', e.payload ->> 'to', '(' || (e.payload ->> 'event') || ')')
                ELSE e.kind END AS what
    FROM book.events e
    WHERE e.task_id IS NOT NULL
    UNION ALL
    SELECT j.tenant, j.task_id, j.created_at, j.role, format('job %s queued on %s', j.id, j.provider)
    FROM book.jobs j
    UNION ALL
    SELECT j.tenant, j.task_id, coalesce(j.finished_at, j.started_at, j.created_at), j.role,
           CASE
           WHEN j.finished_at IS NOT NULL THEN concat_ws(', ',
               format('job %s %s', j.id, coalesce(j.exit_reason, j.state)),
               'after ' || date_trunc('second', j.finished_at - j.started_at)::text
                   || CASE WHEN j.attempt > 1 THEN format(' over %s attempts', j.attempt) ELSE '' END,
               nullif(coalesce((j.usage ->> 'input_tokens')::bigint, 0) + coalesce((j.usage ->> 'output_tokens')::bigint, 0), 0)::text || ' tokens',
               '$' || to_char(nullif((j.usage ->> 'cost_usd')::numeric, 0), 'FM999990.00'),
               'stopped: ' || left(j.result ->> 'stopped', 160))
           WHEN j.state = 'running' THEN format('job %s running, attempt %s', j.id, j.attempt)
           ELSE format('job %s %s (no end time)', j.id, j.state)
           END
    FROM book.jobs j
    WHERE j.state <> 'queued' OR j.finished_at IS NOT NULL;
GRANT SELECT ON book.dash_task_timeline TO grafana_ro;
GRANT SELECT ON book.dash_task_timeline TO nightshift;
