package book_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ilyachakun/nightshift/internal/book"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func exec(t *testing.T, db *sql.DB, q string, args ...any) error {
	t.Helper()
	_, err := db.ExecContext(t.Context(), q, args...)
	return err
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if err := exec(t, db, q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func sqlState(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func TestBook(t *testing.T) {
	db := pgtest.Start(t).Owner
	ctx := t.Context()
	if err := book.Migrate(ctx, db); err != nil {
		t.Fatalf("second run must be a no-op: %v", err)
	}
	var taskID int64
	err := db.QueryRowContext(ctx, `INSERT INTO book.tasks (tenant, repo, title)
		VALUES ('example-solo', 'example/app', 'Add CSV export') RETURNING id`).Scan(&taskID)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO book.events (tenant, task_id, kind, actor) VALUES ('example-solo', $1, 'created', 'system')`, taskID)

	for _, tc := range []struct {
		name string
		run  func(*testing.T, *sql.DB, int64)
	}{
		{"events are append-only even for the owner", eventsAppendOnly},
		{"service role cannot change events", serviceRoleRights},
		{"grafana_ro reads only dashboard views", grafanaRights},
		{"a decision is recorded once per task, kind, sha and actor", decisionOnce},
		{"constraints reject bad values", constraints},
	} {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, db, taskID) })
	}
}

func eventsAppendOnly(t *testing.T, db *sql.DB, _ int64) {
	for _, q := range []string{`UPDATE book.events SET kind = 'forged'`, `DELETE FROM book.events`, `TRUNCATE book.events`} {
		if err := exec(t, db, q); err == nil {
			t.Errorf("%s: succeeded", q)
		}
	}
}

func serviceRoleRights(t *testing.T, db *sql.DB, taskID int64) {
	mustExec(t, db, `SET ROLE nightshift`)
	defer mustExec(t, db, `RESET ROLE`)
	mustExec(t, db, `INSERT INTO book.events (tenant, task_id, kind, actor) VALUES ('example-solo', $1, 'note', 'lead')`, taskID)
	mustExec(t, db, `UPDATE book.tasks SET state = 'grilling', version = version + 1 WHERE id = $1`, taskID)
	expectDenied(t, db, `UPDATE book.events SET kind = 'x'`, `DELETE FROM book.events`, `DELETE FROM book.tasks`)
}

func grafanaRights(t *testing.T, db *sql.DB, _ int64) {
	mustExec(t, db, `SET ROLE grafana_ro`)
	defer mustExec(t, db, `RESET ROLE`)
	var n int
	err := db.QueryRowContext(t.Context(), `SELECT tasks FROM book.dash_task_states WHERE tenant = 'example-solo'`).Scan(&n)
	if err != nil || n != 1 {
		t.Errorf("view: n=%d err=%v", n, err)
	}
	expectDenied(t, db, `SELECT 1 FROM book.tasks`, `SELECT 1 FROM book.events`, `SELECT 1 FROM book.decisions`)
}

func decisionOnce(t *testing.T, db *sql.DB, taskID int64) {
	q := `INSERT INTO book.decisions (task_id, kind, sha, actor) VALUES ($1, 'approve', $2, 'U-OWNER')
		ON CONFLICT ON CONSTRAINT decisions_once DO NOTHING`
	sha := "0123456789abcdef0123456789abcdef01234567"
	mustExec(t, db, q, taskID, sha)
	mustExec(t, db, q, taskID, sha)
	var n int
	err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM book.decisions WHERE task_id = $1`, taskID).Scan(&n)
	if err != nil || n != 1 {
		t.Errorf("decisions: n=%d err=%v", n, err)
	}
}

func constraints(t *testing.T, db *sql.DB, _ int64) {
	for _, q := range []string{
		`UPDATE book.tasks SET head_sha = 'not-a-sha'`,
		`UPDATE book.tasks SET version = 0`,
		`INSERT INTO book.decisions (task_id, kind, actor) VALUES (1, 'self_approve', 'agent')`,
		`INSERT INTO book.outbox (tenant, target, payload, idempotency_key, status) VALUES ('t', 'chat', '{}', 'k', 'lost')`,
		`INSERT INTO book.budgets (task_id, max_iterations, max_tokens, max_duration) VALUES (1, 0, 1, '1 hour')`,
	} {
		if err := exec(t, db, q); sqlState(err) != "23514" { // check_violation
			t.Errorf("%s: got %v, want a check violation", q, err)
		}
	}
}

func expectDenied(t *testing.T, db *sql.DB, queries ...string) {
	t.Helper()
	for _, q := range queries {
		if err := exec(t, db, q); sqlState(err) != "42501" { // insufficient_privilege
			t.Errorf("%s: got %v, want permission denied", q, err)
		}
	}
}
