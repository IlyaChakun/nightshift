package book_test

import (
	"slices"
	"testing"

	"github.com/ilyachakun/nightshift/internal/data/book"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

// TestDashboardTimeline logs in as the dashboard role, the way Grafana does, and reads one
// task's timeline. No text from payloads or job results reaches the view, because the worker
// writes job results (CI only).
func TestDashboardTimeline(t *testing.T) {
	d := pgtest.Start(t)
	db, ctx := d.Owner, t.Context()
	if err := book.DashboardLogin(ctx, db, ""); err != nil {
		t.Fatalf("an empty password is a no-op: %v", err)
	}
	if err := book.DashboardLogin(ctx, db, "test-only"); err != nil {
		t.Fatal(err)
	}
	var task int64
	if err := db.QueryRowContext(ctx, `INSERT INTO book.tasks (tenant, repo, title) VALUES ('example-solo', 'example/app', 't') RETURNING id`).Scan(&task); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO book.events (tenant, task_id, kind, actor, payload, created_at) VALUES
		('example-solo', $1, 'task.moved', 'system', '{"from":"queued","to":"building","event":"lead_started"}', '2026-11-26 10:00:00Z'),
		('example-solo', $1, 'note', 'lead', '{"text":"agent prose"}', '2026-11-26 10:00:01Z')`, task)
	mustExec(t, db, `INSERT INTO book.jobs (tenant, task_id, role, provider, payload, state, attempt, exit_reason, result, usage, created_at, started_at, finished_at) VALUES
		('example-solo', $1, 'builder', 'p1', '{}', 'failed', 2, 'budget', '{"stopped":"repeated call","error":"harness text"}',
		 '{"input_tokens":1000,"output_tokens":234,"cost_usd":0.5}', '2026-11-26 10:01:00Z', '2026-11-26 10:02:00Z', '2026-11-26 10:12:30Z'),
		('example-solo', $1, 'verifier', 'p1', '{}', 'canceled', 0, NULL, NULL, NULL, '2026-11-26 10:20:00Z', NULL, NULL),
		('example-solo', $1, 'verifier', 'p1', '{}', 'queued', 0, NULL, NULL, NULL, '2026-11-26 10:30:00Z', NULL, NULL)`, task)

	pool := d.Pool(t, book.DashboardRole)
	rows, err := pool.Query(ctx, `SELECT who, regexp_replace(what, 'job [0-9]+', 'job N') FROM book.dash_task_timeline WHERE task_id = $1 ORDER BY at, what`, task)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var who, what string
		if err := rows.Scan(&who, &what); err != nil {
			t.Fatal(err)
		}
		got = append(got, who+": "+what)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"system: queued -> building (lead_started)",
		"lead: note",
		"builder: job N queued on p1",
		"builder: job N budget, after 00:10:30 over 2 attempts, 1234 tokens, $0.50", // no "stopped" text
		"verifier: job N canceled (no end time)",                                    // same time as its queued line, which sorts after it
		"verifier: job N queued on p1",
		"verifier: job N queued on p1",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("timeline:\n got %q\nwant %q", got, want)
	}
	for _, q := range []string{`SELECT 1 FROM book.jobs`, `SELECT 1 FROM book.events`} {
		if _, err := pool.Exec(ctx, q); sqlState(err) != "42501" {
			t.Errorf("%s: got %v, want permission denied", q, err)
		}
	}
}
