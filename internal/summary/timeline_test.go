package summary_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/summary"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func at(min int) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: since.Add(time.Duration(min) * time.Minute), Valid: true}
}

// The timeline puts moves and jobs in time order, and says why a run did not finish.
func TestBuildTimeline(t *testing.T) {
	t.Parallel()
	events := []bookdb.BookEvent{
		{Kind: "task.moved", Actor: "system", Payload: []byte(`{"from":"queued","to":"building","event":"start"}`), CreatedAt: at(0)},
		{Kind: "turn.started", Actor: "builder", Payload: []byte(`{}`), CreatedAt: at(2)},
	}
	jobs := []bookdb.BookJob{
		{
			ID: 7, Role: "builder", Provider: "claude-sub", State: "done", CreatedAt: at(1), StartedAt: at(2), FinishedAt: at(12),
			ExitReason: pgtype.Text{String: "budget", Valid: true}, Result: []byte(`{"stopped":"loop: Bash called 6 times in a row with the same input"}`),
			Usage: []byte(`{"input_tokens":900,"output_tokens":100,"cost_usd":0.5}`),
		},
		{ID: 8, Role: "builder", Provider: "codex-api", State: "running", Attempt: 1, CreatedAt: at(13), StartedAt: at(14)},
	}
	got := summary.FormatTimeline(3, summary.BuildTimeline(events, jobs), time.UTC)
	want := []string{
		"2026-11-24 22:00:00  system     queued -> building (start)",
		"2026-11-24 22:01:00  builder    job 7 queued on claude-sub",
		"2026-11-24 22:02:00  builder    turn.started",
		"2026-11-24 22:12:00  builder    job 7 budget, after 10m0s, 1000 tokens, $0.50: loop: Bash called 6 times in a row with the same input",
		"2026-11-24 22:13:00  builder    job 8 queued on codex-api",
		"2026-11-24 22:14:00  builder    job 8 running, attempt 1",
	}
	if lines := strings.Split(strings.TrimSpace(got), "\n")[1:]; strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("timeline:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
	if got := summary.FormatTimeline(3, nil, time.UTC); !strings.Contains(got, "no events yet") {
		t.Errorf("empty: %q", got)
	}
}

func TestLongErrorsAreClipped(t *testing.T) {
	t.Parallel()
	jobs := []bookdb.BookJob{{
		ID: 1, Role: "lead", Provider: "p", State: "failed", CreatedAt: at(0), FinishedAt: at(1),
		ExitReason: pgtype.Text{String: "error", Valid: true}, Result: []byte(`{"error":"` + strings.Repeat("x", 500) + `"}`),
	}}
	entries := summary.BuildTimeline(nil, jobs)
	if last := entries[len(entries)-1].What; len(last) > 170 || !strings.HasSuffix(last, "...") {
		t.Errorf("not clipped: %d chars", len(last))
	}
}

// T3: the timeline comes from the book, and a task of another tenant is not shown.
func TestTimelineFromTheBook(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	q := bookdb.New(pool)
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{Tenant: "example-solo", TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: "task.moved", Actor: "system", Payload: []byte(`{"from":"queued","to":"building","event":"start"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.EnqueueJob(ctx, bookdb.EnqueueJobParams{Tenant: "example-solo", TaskID: task.ID, Role: "verifier", Provider: "claude-sub", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	entries, err := summary.Timeline(ctx, pool, "example-solo", task.ID)
	if err != nil || len(entries) != 2 || !strings.Contains(entries[1].What, "queued on claude-sub") {
		t.Fatalf("entries %+v, err %v", entries, err)
	}
	if _, err := summary.Timeline(ctx, pool, "example-team", task.ID); !errors.Is(err, summary.ErrNoTask) {
		t.Fatalf("another tenant's task: %v", err)
	}
}
