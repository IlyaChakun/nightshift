package book_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

// The queries run as the nightshift service role, the way serve will run them.
func serviceQueries(t *testing.T) (*bookdb.Queries, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Start(t).ServicePool(t)
	return bookdb.New(pool), pool
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func TestQueries(t *testing.T) {
	q, pool := serviceQueries(t)
	ctx := context.Background()
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{
		Tenant: "example-solo", Repo: "example/app", Title: "Add CSV export", ThreadRef: text("C1/1.0"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "idea" || task.Version != 1 {
		t.Fatalf("new task: %+v", task)
	}
	t.Run("transition is a compare-and-set", func(t *testing.T) { transitionCAS(t, q, task) })
	t.Run("decisions and outbox are idempotent", func(t *testing.T) { idempotent(t, q, task.ID) })
	t.Run("claim skips rows locked by another worker", func(t *testing.T) { claimSkipLocked(t, q, pool) })
}

func transitionCAS(t *testing.T, q *bookdb.Queries, task bookdb.BookTask) {
	ctx := context.Background()
	moved, err := q.TransitionTask(ctx, bookdb.TransitionTaskParams{
		ID: task.ID, FromState: "idea", ToState: "grilling", Version: task.Version,
	})
	if err != nil || moved.Version != 2 || moved.State != "grilling" {
		t.Fatalf("first transition: %+v, %v", moved, err)
	}
	// The same read again lost the race: no row, nothing changed.
	_, err = q.TransitionTask(ctx, bookdb.TransitionTaskParams{
		ID: task.ID, FromState: "idea", ToState: "spec", Version: task.Version,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale transition: got %v, want no rows", err)
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	_, err = q.TransitionTask(ctx, bookdb.TransitionTaskParams{
		ID: task.ID, FromState: "grilling", ToState: "spec", Version: 2, HeadSha: text(sha),
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("wrong head_sha: got %v, want no rows", err)
	}
}

func idempotent(t *testing.T, q *bookdb.Queries, taskID int64) {
	ctx := context.Background()
	d := bookdb.RecordDecisionParams{TaskID: taskID, Kind: "approve", Sha: "", Actor: "U-OWNER"}
	if _, err := q.RecordDecision(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RecordDecision(ctx, d); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("repeated decision: got %v, want no rows", err)
	}
	o := bookdb.EnqueueOutboxParams{
		Tenant: "example-solo", TaskID: pgtype.Int8{Int64: taskID, Valid: true},
		Target: "chat", Payload: []byte(`{"text":"spec ready"}`), IdempotencyKey: "task-1/spec-card/v1",
	}
	if _, err := q.EnqueueOutbox(ctx, o); err != nil {
		t.Fatal(err)
	}
	if _, err := q.EnqueueOutbox(ctx, o); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("repeated enqueue: got %v, want no rows", err)
	}
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: "example-solo", TaskID: pgtype.Int8{Int64: taskID, Valid: true},
		Kind: "decision", Actor: "U-OWNER", Payload: []byte(`{"kind":"approve"}`),
	}); err != nil {
		t.Fatal(err)
	}
}

func claimSkipLocked(t *testing.T, q *bookdb.Queries, pool *pgxpool.Pool) {
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	first, err := q.WithTx(tx).ClaimOutbox(ctx, 10)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim: %d rows, %v", len(first), err)
	}
	// A second worker sees nothing while the first holds the lock.
	second, err := q.ClaimOutbox(ctx, 10)
	if err != nil || len(second) != 0 {
		t.Fatalf("second claim: %d rows, %v", len(second), err)
	}
	retryAt := pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true}
	if err := q.WithTx(tx).MarkOutboxRetry(ctx, bookdb.MarkOutboxRetryParams{
		ID: first[0].ID, LastError: text("chat API: 429"), NextAttemptAt: retryAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if due, err := q.ClaimOutbox(ctx, 10); err != nil || len(due) != 0 {
		t.Fatalf("after retry: %d rows due, %v", len(due), err)
	}
}
