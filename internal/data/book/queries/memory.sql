-- name: AddFinding :one
INSERT INTO book.findings (tenant, task_id, job_id, rule_id, category, severity, layer_caught,
                           layer_expected, message, path, start_line, end_line, fingerprint, ci_checks)
VALUES (@tenant, @task_id, @job_id, @rule_id, @category, @severity, @layer_caught,
        @layer_expected, @message, @path, @start_line, @end_line, @fingerprint, sqlc.narg(ci_checks)::text[])
RETURNING id;

-- name: ListTaskFindings :many
SELECT * FROM book.finding_states WHERE task_id = @task_id ORDER BY id;

-- name: ListVerifierFindingsToRecheck :many
SELECT f.* FROM book.finding_states AS f
JOIN book.jobs AS j ON j.id = f.job_id
WHERE f.task_id = @task_id AND f.status = 'handed_off' AND f.layer_caught = 'L5' AND j.role = 'verifier'
ORDER BY f.id;

-- name: HandOffFindings :exec
UPDATE book.findings SET handoff_job_id = @builder_job_id
WHERE id = ANY(@ids::bigint[]) AND status = 'open';

-- Match exact fingerprints or the same rule and source location. A moved line or
-- changed message must not turn a still-reported problem into evidence of a fix.
-- name: ResolveJobFindings :exec
UPDATE book.findings AS old SET status = 'fixed'
FROM book.jobs AS source, book.jobs AS checked
WHERE old.task_id = @task_id AND old.status = 'open' AND old.job_id = source.id
  AND checked.id = @checked_job_id AND checked.task_id = old.task_id
  AND checked.state = 'done' AND checked.finding_coverage IS NOT NULL
  AND source.role = checked.role
  AND (
       (checked.role = 'builder' AND old.handoff_job_id = checked.id
         AND old.layer_caught IN ('L1', 'L2') AND old.rule_id <> 'constitution.protected'
         AND checked.finding_coverage NOT LIKE 'partial:%'
         AND source.finding_coverage IN (checked.finding_coverage, 'partial:' || checked.finding_coverage))
    OR (checked.role = 'verifier' AND old.handoff_job_id < checked.id
         AND old.id = ANY(@target_ids::bigint[])
         AND old.layer_caught = 'L5' AND checked.finding_coverage = 'verifier')
  )
  AND NOT EXISTS (
    SELECT 1 FROM book.findings AS current
    WHERE current.job_id = checked.id
      AND ((old.fingerprint <> '' AND current.fingerprint = old.fingerprint)
        OR (current.layer_caught = old.layer_caught
          AND current.rule_id = old.rule_id AND current.category = old.category
          AND current.path IS NOT DISTINCT FROM old.path))
  );

-- name: ResolveCIFindings :exec
UPDATE book.findings SET status = 'fixed'
WHERE task_id = @task_id AND status = 'open' AND handoff_job_id IS NOT NULL
  AND layer_caught = 'L3' AND rule_id = 'ci.failed'
  AND cardinality(ci_checks) > 0 AND ci_checks <@ @passed_checks::text[];


-- name: AddNote :one
INSERT INTO book.notes (tenant, task_id, job_id, role, kind, body, source)
VALUES (@tenant, @task_id, @job_id, @role, @kind, @body, @source)
RETURNING id;

-- name: ListNotes :many
SELECT * FROM book.notes WHERE tenant = @tenant AND status = @status ORDER BY id;
