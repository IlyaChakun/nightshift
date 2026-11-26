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

func at(minutes int) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: since.Add(time.Duration(minutes) * time.Minute), Valid: true}
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

// The timeline shows the model and effort a job asked for and the model the harness ran (L5).
func TestTimelineShowsModelAndEffort(t *testing.T) {
	t.Parallel()
	jobs := []bookdb.BookJob{{
		ID: 4, Role: "lead", Provider: "claude-sub", State: "done", CreatedAt: at(0), StartedAt: at(1), FinishedAt: at(3),
		ExitReason: pgtype.Text{String: "done", Valid: true},
		Payload:    []byte(`{"model":"claude-opus-5-5","effort":"high"}`),
		Usage:      []byte(`{"input_tokens":10,"output_tokens":5,"cache_read_tokens":2000,"model":"claude-opus-5-5"}`),
	}, {ID: 5, Role: "lead", Provider: "p", State: "queued", CreatedAt: at(4), Payload: []byte(`{"effort":"high"}`)}}
	got := summary.FormatTimeline(1, summary.BuildTimeline(nil, jobs), time.UTC)
	for _, want := range []string{
		"job 4 queued on claude-sub (claude-opus-5-5, high)",
		"job 4 done, after 2m0s, ran claude-opus-5-5, 15 tokens, 2000 read from cache",
		"job 5 queued on p (high)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("timeline lacks %q:\n%s", want, got)
		}
	}
}

// Regression: a canceled job has no end time; the timeline still says it ended, and a
// requeued job's time says it covers every attempt.
func TestCanceledAndRetriedJobs(t *testing.T) {
	t.Parallel()
	jobs := []bookdb.BookJob{
		{ID: 1, Role: "builder", Provider: "p", State: "canceled", CreatedAt: at(0), StartedAt: at(1)},
		{ID: 2, Role: "builder", Provider: "p", State: "done", Attempt: 2, CreatedAt: at(2), StartedAt: at(3), FinishedAt: at(9), ExitReason: pgtype.Text{String: "done", Valid: true}},
	}
	got := summary.FormatTimeline(1, summary.BuildTimeline(nil, jobs), time.UTC)
	for _, want := range []string{"job 1 canceled (no end time)", "job 2 done, after 6m0s over 2 attempts"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
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
