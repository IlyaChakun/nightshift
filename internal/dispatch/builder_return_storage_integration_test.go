package dispatch_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func TestBuilderReturnRetriesItsRecord(t *testing.T) {
	db := pgtest.Start(t)
	r := newRigOnBook(t, db)
	r.advance()
	r.finish(nil, red("the exported row count is incorrect"))
	ctx := t.Context()
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.jobs
		SET result = jsonb_set(result, '{handoff}', '{"done":["the export"]}')
		WHERE id = $1`, r.lastJob().ID); err != nil {
		t.Fatal(err)
	}

	// Refuse just this event in this test's private database.
	_, err := db.Owner.ExecContext(ctx, `ALTER TABLE book.events
		ADD CONSTRAINT fixture_unavailable_return CHECK (kind <> 'builder.returned')`)
	if err != nil {
		t.Fatal(err)
	}
	err = r.c.Advance(ctx, r.tenant, r.task)
	var storageErr *pgconn.PgError
	if !errors.As(err, &storageErr) || storageErr.Code != "23514" || storageErr.ConstraintName != "fixture_unavailable_return" {
		t.Errorf("failed return record: got %v, want original storage constraint error", err)
	}
	check := func(stage string, consumed bool, wantJobs int, wantCount int64) {
		t.Helper()
		jobs, err := r.q.ListTaskJobs(ctx, bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "builder"})
		if err != nil || len(jobs) == 0 {
			t.Fatalf("%s: list builder jobs: %d, %v", stage, len(jobs), err)
		}
		n, err := r.q.CountTaskEvents(ctx, bookdb.CountTaskEventsParams{TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: "builder.returned"})
		if err != nil {
			t.Fatal(err)
		}
		if jobs[0].ConsumedAt.Valid != consumed || len(jobs) != wantJobs || n != wantCount {
			t.Errorf("%s: consumed=%t jobs=%d returns=%d, want %t/%d/%d", stage, jobs[0].ConsumedAt.Valid, len(jobs), n, consumed, wantJobs, wantCount)
		}
		handoffs, err := r.q.CountTaskEvents(ctx, bookdb.CountTaskEventsParams{TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: "handoff"})
		if err != nil || handoffs != wantCount {
			t.Errorf("%s: handoffs = %d, err = %v, want %d", stage, handoffs, err, wantCount)
		}
	}
	check("after storage error", false, 1, 0)
	if _, err := db.Owner.ExecContext(ctx, `ALTER TABLE book.events DROP CONSTRAINT fixture_unavailable_return`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Owner.ExecContext(ctx, `ALTER TABLE book.jobs
		ADD CONSTRAINT fixture_unavailable_next_job CHECK (role <> 'builder' OR state <> 'queued')`); err != nil {
		t.Fatal(err)
	}
	err = r.c.Advance(ctx, r.tenant, r.task)
	if !errors.As(err, &storageErr) || storageErr.ConstraintName != "fixture_unavailable_next_job" {
		t.Fatalf("next job write returned %v, want the original storage error", err)
	}
	check("after enqueue error", true, 1, 1)
	if _, err := db.Owner.ExecContext(ctx, `ALTER TABLE book.jobs DROP CONSTRAINT fixture_unavailable_next_job`); err != nil {
		t.Fatal(err)
	}
	r.advance()
	check("after retry", true, 2, 1)
	r.advance()
	check("after subsequent tick", true, 2, 1)
}
