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
