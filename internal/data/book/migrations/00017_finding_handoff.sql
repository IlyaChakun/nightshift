-- +goose Up
ALTER TABLE book.findings ADD COLUMN handoff_job_id bigint;
ALTER TABLE book.jobs ADD COLUMN finding_coverage text;

-- Expose delivery alongside the original resolution states.
CREATE VIEW book.finding_states AS
SELECT id, tenant, task_id, job_id, rule_id, category, severity, layer_caught, layer_expected,
       message, path, start_line, end_line, fingerprint,
       (CASE WHEN status = 'open' AND handoff_job_id IS NOT NULL THEN 'handed_off' ELSE status END)::text AS status,
       created_at, handoff_job_id
FROM book.findings;
GRANT SELECT ON book.finding_states TO nightshift;
