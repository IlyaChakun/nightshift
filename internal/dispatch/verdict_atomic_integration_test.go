package dispatch_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func TestVerifierVerdictRollsBackAndReplaysOnce(t *testing.T) {
	db := pgtest.Start(t)
	r := newRigOnBook(t, db)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	r.toReview("feature.txt")
	ctx := t.Context()
	restarted := *r.c
	concurrentAdvance := func() {
		t.Helper()
		stale := r.task
		errs := make(chan error, 2)
		go func() { errs <- r.c.Advance(ctx, r.tenant, stale) }()
		go func() { errs <- restarted.Advance(ctx, r.tenant, stale) }()
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
	}
	concurrentAdvance() // Only one open Verifier job for this review.
	r.verdict(`{"verdict":"return","summary":"rework the export","raise_blast_radius":{"level":"high","reason":"changes access"},"findings":[{"rule_id":"export","category":"code","severity":"block","layer_caught":"L5","message":"one row is missing"}]}`)

	// Fail the last write, after the transaction has recorded risk and moved the task.
	execVerdictFixture(t, db, `ALTER TABLE book.events ADD CONSTRAINT fixture_reject_review_move
		CHECK (kind <> 'task.moved' OR payload->>'from' <> 'review')`)
	execVerdictFixture(t, db, `ALTER TABLE book.events ADD CONSTRAINT fixture_reject_verifier_risk
		CHECK (kind <> 'review.raised')`)
	check := func(state string, consumed bool, count int) {
		t.Helper()
		r.reload()
		jobs, err := r.q.ListTaskJobs(ctx, bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "verifier"})
		if err != nil || len(jobs) != 1 || jobs[0].ConsumedAt.Valid != consumed {
			t.Fatalf("Verifier jobs = %+v, err = %v, want one job, consumed = %t", jobs, err, consumed)
		}
		found, err := r.q.ListJobFindings(ctx, pgtype.Int8{Int64: jobs[0].ID, Valid: true})
		if err != nil || len(found) != 1 {
			t.Fatalf("finding count = %d, err = %v, want one finding from job completion", len(found), err)
		}
		raised, err := r.q.CountTaskEvents(ctx, bookdb.CountTaskEventsParams{
			TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: "review.raised",
		})
		if err != nil || raised != int64(count) || r.task.State != state || int(r.task.Returns) != count {
			t.Fatalf("state = %s, returns = %d, risk events = %d, err = %v", r.task.State, r.task.Returns, raised, err)
		}
	}
	expectError := func(constraint string) {
		t.Helper()
		err := r.c.Advance(ctx, r.tenant, r.task)
		var storageErr *pgconn.PgError
		if !errors.As(err, &storageErr) || storageErr.ConstraintName != constraint {
			t.Fatalf("review returned %v, want the original write error for %s", err, constraint)
		}
		check("review", false, 0)
	}
	expectError("fixture_reject_verifier_risk")
	execVerdictFixture(t, db, "ALTER TABLE book.events DROP CONSTRAINT fixture_reject_verifier_risk")
	expectError("fixture_reject_review_move")
	execVerdictFixture(t, db, "ALTER TABLE book.events DROP CONSTRAINT fixture_reject_review_move")
	stale := r.task
	concurrentAdvance()
	check("building", true, 1)
	if err := restarted.Advance(ctx, r.tenant, stale); err != nil {
		t.Fatal(err)
	}
	check("building", true, 1)
}

func execVerdictFixture(t *testing.T, db *pgtest.DB, query string) {
	t.Helper()
	if _, err := db.Owner.ExecContext(t.Context(), query); err != nil {
		t.Fatal(err)
	}
}

func TestVerifierBudgetIgnoresPriorPassAndStaleReturn(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	role := r.c.Roles["verifier"]
	role.Limits.MaxReturns = 1
	r.c.Roles["verifier"] = role
	r.toReview("feature.txt")
	r.advance()
	r.verdict(`{"verdict":"pass","summary":"ok"}`)
	ctx := t.Context()
	// An older dispatcher could consume a pass and stop before moving the task.
	if _, err := r.q.ConsumeJob(ctx, r.verifierJob().ID); err != nil {
		t.Fatal(err)
	}
	r.advance()
	r.verdict(`{"verdict":"return","summary":"outdated patch"}`)
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.jobs SET payload = jsonb_set(payload, '{base_sha}', '"old-head"')
		WHERE id = $1`, r.verifierJob().ID); err != nil {
		t.Fatal(err)
	}
	r.advance()
	if r.task.State != "review" || r.task.Returns != 0 || !r.verifierJob().ConsumedAt.Valid {
		t.Fatalf("stale review moved the task to %s or spent its returns (%d)", r.task.State, r.task.Returns)
	}
	r.advance()
	r.verdict(`{"verdict":"return","summary":"fix the export"}`)
	r.advance()
	if r.task.State != "building" || r.task.Returns != 1 {
		t.Fatalf("current return moved to %s with %d returns, want building with one return", r.task.State, r.task.Returns)
	}
}
