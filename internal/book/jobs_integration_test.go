package book_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

func at(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func hash(c string) pgtype.Text { return text(strings.Repeat(c, 64)) }

var jobsNow = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

type jobsFixture struct {
	q             *bookdb.Queries
	task          bookdb.BookTask
	first, second bookdb.BookJob
}

func (f *jobsFixture) claim(t *testing.T, token pgtype.Text) (bookdb.BookJob, error) {
	t.Helper()
	return f.q.ClaimJob(t.Context(), bookdb.ClaimJobParams{
		WorkerID: text("w1"), TokenSha256: token, LeaseUntil: at(jobsNow.Add(time.Minute)), Now: at(jobsNow),
	})
}

func TestJobs(t *testing.T) {
	q, _ := serviceQueries(t)
	f := &jobsFixture{q: q}
	var err error
	f.task, err = q.CreateTask(t.Context(), bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Jobs"})
	if err != nil {
		t.Fatal(err)
	}
	f.first, f.second = f.enqueue(t), f.enqueue(t)
	if got, err := f.claim(t, hash("a")); err != nil || got.ID != f.first.ID || got.State != "running" || got.Attempt != 1 {
		t.Fatalf("claim: %+v, %v", got, err)
	}
	t.Run("a token works only for its own job and lease", f.tokenScope)
	t.Run("complete happens once", f.completeOnce)
	t.Run("an expired lease is requeued, then failed", f.expiredLease)
	t.Run("artifacts and sessions", f.artifactsAndSessions)
}

func (f *jobsFixture) enqueue(t *testing.T) bookdb.BookJob {
	t.Helper()
	j, err := f.q.EnqueueJob(t.Context(), bookdb.EnqueueJobParams{
		Tenant: "example-solo", TaskID: f.task.ID, Role: "builder", Provider: "claude-api", Payload: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func (f *jobsFixture) tokenScope(t *testing.T) {
	for name, p := range map[string]bookdb.GetLeasedJobParams{
		"wrong token":   {ID: f.first.ID, TokenSha256: hash("b"), Now: at(jobsNow)},
		"expired lease": {ID: f.first.ID, TokenSha256: hash("a"), Now: at(jobsNow.Add(2 * time.Minute))},
		"other job":     {ID: f.second.ID, TokenSha256: hash("a"), Now: at(jobsNow)},
	} {
		if _, err := f.q.GetLeasedJob(t.Context(), p); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func (f *jobsFixture) completeOnce(t *testing.T) {
	done := bookdb.CompleteJobParams{
		ID: f.first.ID, TokenSha256: hash("a"), State: "done", ExitReason: text("done"),
		Result: []byte(`{"ok":true}`), Usage: []byte(`{}`), Now: at(jobsNow),
	}
	if j, err := f.q.CompleteJob(t.Context(), done); err != nil || j.State != "done" || j.TokenSha256.Valid {
		t.Fatalf("complete: %+v, %v", j, err)
	}
	if _, err := f.q.CompleteJob(t.Context(), done); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("second complete: %v", err)
	}
}

func (f *jobsFixture) expiredLease(t *testing.T) {
	ctx := t.Context()
	if _, err := f.claim(t, hash("c")); err != nil {
		t.Fatal(err)
	}
	later := at(jobsNow.Add(5 * time.Minute))
	ids, err := f.q.RequeueExpiredJobs(ctx, bookdb.RequeueExpiredJobsParams{Now: later, MaxAttempts: 2})
	if err != nil || len(ids) != 1 || ids[0] != f.second.ID {
		t.Fatalf("requeue: %v, %v", ids, err)
	}
	if j, err := f.claim(t, hash("d")); err != nil || j.Attempt != 2 {
		t.Fatalf("second claim: %+v, %v", j, err)
	}
	ids, err = f.q.FailExpiredJobs(ctx, bookdb.FailExpiredJobsParams{Now: later, MaxAttempts: 2})
	if err != nil || len(ids) != 1 {
		t.Fatalf("fail: %v, %v", ids, err)
	}
	if _, err := f.claim(t, hash("e")); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("queue should be empty: %v", err)
	}
}

func (f *jobsFixture) artifactsAndSessions(t *testing.T) {
	ctx := t.Context()
	a := bookdb.AddArtifactParams{JobID: f.first.ID, Path: "result.json", Sha256: strings.Repeat("f", 64), Bytes: 12}
	for range 2 { // a repeat is dropped
		if err := f.q.AddArtifact(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if list, err := f.q.ListJobArtifacts(ctx, f.first.ID); err != nil || len(list) != 1 {
		t.Fatalf("artifacts: %+v, %v", list, err)
	}
	for _, id := range []string{"s1", "s2"} {
		p := bookdb.SaveHarnessSessionParams{TaskID: f.task.ID, Role: "builder", Provider: "claude-api", SessionID: id}
		if err := f.q.SaveHarnessSession(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	s, err := f.q.GetHarnessSession(ctx, bookdb.GetHarnessSessionParams{TaskID: f.task.ID, Role: "builder"})
	if err != nil || s.SessionID != "s2" {
		t.Fatalf("session: %+v, %v", s, err)
	}
}
