package summary_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/summary"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

var since = time.Date(2026, 11, 24, 22, 0, 0, 0, time.UTC)

func TestFormat(t *testing.T) {
	t.Parallel()
	got := summary.Summary{
		Since: since, Finished: 3, Active: 2,
		Waiting: map[flow.State]int64{flow.Ready: 1, flow.HumanReview: 2},
		Merged:  4, WithoutChanges: 3, Median: 90 * time.Minute,
	}.Format()
	for _, want := range []string{"3 finished, 2 in work", "Waiting for you: 3 (2 human review, 1 ready)", "3 of 4 (75%)", "1.5 h"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	empty := summary.Summary{Since: since, Waiting: map[flow.State]int64{}}.Format()
	if !strings.Contains(empty, "Nothing waits for you") || !strings.Contains(empty, "No merged tasks yet") {
		t.Errorf("empty summary:\n%s", empty)
	}
}

// TestBuildFromTheBook runs the SQL against a real book (CI only).
func TestBuildFromTheBook(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	q := bookdb.New(pool)
	m := &flow.Mover{Pool: pool}
	mk := func(title string) bookdb.BookTask {
		task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: title})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	// One task released without changes; one waiting in the grilling state.
	done := mk("done")
	for _, e := range []flow.Event{flow.LeadStarted, flow.DocsDrafted, flow.DocLoopPassed} {
		if _, err := m.Apply(ctx, flow.Move{TaskID: done.ID, Event: e, Actor: "system", Profile: flow.Solo}); err != nil {
			t.Fatalf("%s: %v", e, err)
		}
	}
	_ = mk("idle")
	// A released task is simplest to stage directly: the summary reads events, not states.
	payload := []byte(`{"to":"released","event":"release_done"}`)
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{Tenant: "example-solo", TaskID: pgtype.Int8{Int64: done.ID, Valid: true}, Kind: "task.moved", Actor: "system", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	s, err := summary.Build(ctx, pool, "example-solo", since.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if s.Finished != 1 || s.Merged != 1 || s.WithoutChanges != 1 || s.Median < 0 {
		t.Fatalf("summary: %+v", s)
	}
	// A request for changes on the task takes it out of the "without changes" share.
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{Tenant: "example-solo", TaskID: pgtype.Int8{Int64: done.ID, Valid: true}, Kind: "task.moved", Actor: "U1", Payload: []byte(`{"to":"building","event":"request_changes"}`)}); err != nil {
		t.Fatal(err)
	}
	if s, err = summary.Build(ctx, pool, "example-solo", since.Add(-24*time.Hour)); err != nil || s.WithoutChanges != 0 || s.Merged != 1 {
		t.Fatalf("after request changes: %+v %v", s, err)
	}
}

func TestFormatLimits(t *testing.T) {
	t.Parallel()
	got := summary.Summary{
		Since: since, Waiting: map[flow.State]int64{},
		LimitHits: map[string]int64{"claude-sub": 2, "deepseek": 1}, FallbackJobs: 3,
	}.Format()
	if want := "Model limits: hit by claude-sub 2, deepseek 1; 3 jobs ran on a fallback provider."; !strings.Contains(got, want) {
		t.Errorf("missing %q in:\n%s", want, got)
	}
	if quiet := (summary.Summary{Since: since, Waiting: map[flow.State]int64{}}).Format(); strings.Contains(quiet, "Model limits") {
		t.Errorf("a quiet night mentions limits:\n%s", quiet)
	}
}

// TestBuildCountsLimitsAndFallbackJobs reads the job table of a real book (CI only).
func TestBuildCountsLimitsAndFallbackJobs(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	q := bookdb.New(pool)
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range []struct{ provider, exit string }{{"claude-sub", "rate_limit"}, {"claude-sub", "rate_limit"}, {"deepseek", "done"}, {"claude-sub", "done"}} {
		job, err := q.EnqueueJob(ctx, bookdb.EnqueueJobParams{Tenant: "example-solo", TaskID: task.ID, Role: "builder", Provider: j.provider, Payload: []byte(`{}`)})
		if err != nil {
			t.Fatal(err)
		}
		state := "done"
		if j.exit == "rate_limit" {
			state = "failed"
		}
		if _, err := pool.Exec(ctx, `UPDATE book.jobs SET state = $2, exit_reason = $3, finished_at = now() WHERE id = $1`, job.ID, state, j.exit); err != nil {
			t.Fatal(err)
		}
	}
	s, err := summary.Build(ctx, pool, "example-solo", time.Now().Add(-time.Hour), "deepseek")
	if err != nil {
		t.Fatal(err)
	}
	if s.LimitHits["claude-sub"] != 2 || s.FallbackJobs != 1 {
		t.Fatalf("limits %v, fallback jobs %d", s.LimitHits, s.FallbackJobs)
	}
}

func TestDecisionsListsOnlyDecisionsOfTheTenantInTheWindow(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	q := bookdb.New(pool)
	mk := func(tenant, title string) int64 {
		task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: tenant, Repo: "example/app", Title: title})
		if err != nil {
			t.Fatal(err)
		}
		return task.ID
	}
	move := func(tenant string, id int64, actor, payload string) {
		if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{Tenant: tenant, TaskID: pgtype.Int8{Int64: id, Valid: true}, Kind: "task.moved", Actor: actor, Payload: []byte(payload)}); err != nil {
			t.Fatal(err)
		}
	}
	mine, other := mk("example-solo", "mine"), mk("example-team", "other tenant")
	move("example-solo", mine, "U1", `{"from":"spec","to":"queued","event":"approve"}`)
	move("example-solo", mine, "system", `{"from":"queued","to":"building","event":"lead_started"}`) // routine: not a decision
	move("example-team", other, "U2", `{"from":"spec","to":"canceled","event":"cancel"}`)
	got, err := summary.Decisions(ctx, pool, "example-solo", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Event != "approve" || got[0].Actor != "U1" || got[0].From != "spec" || got[0].To != "queued" || got[0].Title != "mine" {
		t.Fatalf("decisions = %+v", got)
	}
	if later, _ := summary.Decisions(ctx, pool, "example-solo", time.Now().Add(time.Hour)); len(later) != 0 {
		t.Fatalf("a window in the future holds nothing: %+v", later)
	}
}
