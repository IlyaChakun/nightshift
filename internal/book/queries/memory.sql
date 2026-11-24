-- name: AddFinding :one
INSERT INTO book.findings (tenant, task_id, job_id, rule_id, category, severity, layer_caught,
                           layer_expected, message, path, start_line, end_line, fingerprint)
VALUES (@tenant, @task_id, @job_id, @rule_id, @category, @severity, @layer_caught,
        @layer_expected, @message, @path, @start_line, @end_line, @fingerprint)
RETURNING id;

-- name: ListTaskFindings :many
SELECT * FROM book.findings WHERE task_id = @task_id ORDER BY id;

-- name: SetFindingStatus :one
UPDATE book.findings SET status = @status WHERE id = @id RETURNING *;

-- name: AddNote :one
INSERT INTO book.notes (tenant, task_id, job_id, role, kind, body, source)
VALUES (@tenant, @task_id, @job_id, @role, @kind, @body, @source)
RETURNING id;

-- name: ListNotes :many
SELECT * FROM book.notes WHERE tenant = @tenant AND status = @status ORDER BY id;
