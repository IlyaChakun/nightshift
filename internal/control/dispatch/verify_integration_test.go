package dispatch_test

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/control/jobserver"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func (r *rig) withVerifier() {
	r.t.Helper()
	r.c.Roles["verifier"] = config.Role{Name: "verifier", Access: "read_only", OutputSchema: "schemas/verdict.json", Limits: config.RoleLimits{MaxReturns: 2}}
	r.c.Router.Chains["verifier"] = []string{"fake"}
}

func (r *rig) verifierJob() bookdb.BookJob {
	r.t.Helper()
	jobs, err := r.q.ListTaskJobs(r.t.Context(), bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "verifier"})
	if err != nil || len(jobs) == 0 {
		r.t.Fatalf("verifier jobs: %v %v", jobs, err)
	}
	return jobs[len(jobs)-1]
}

func (r *rig) verdict(result string) {
	r.t.Helper()
	const workerToken = "worker-fixture"
	server := &jobserver.Server{Pool: r.c.Pool, WorkerTokenSHA256: jobserver.HashToken(workerToken)}
	api := httptest.NewServer(server.Handler())
	defer api.Close()
	client := &jobsapi.Client{BaseURL: api.URL, WorkerToken: workerToken, HTTP: api.Client()}
	ctx := r.t.Context()
	job := r.verifierJob()
	claim, err := client.Claim(ctx, "fixture", 0)
	if err != nil || claim.JobID != strconv.FormatInt(job.ID, 10) || claim.Job.Role != "verifier" {
		r.t.Fatalf("claim: got job %q, role %q, err %v", claim.JobID, claim.Job.Role, err)
	}
	if err := client.Complete(ctx, claim.JobID, claim.JobToken, jobsapi.CompleteRequest{ExitReason: "done", Result: json.RawMessage(result)}); err != nil {
		r.t.Fatal(err)
	}
}

// passReview supplies a completed review when the test exercises a later gate.
func (r *rig) passReview() {
	r.t.Helper()
	r.withVerifier()
	r.advance()
	r.verdict(`{"verdict":"pass","summary":"ok"}`)
	r.advance()
}

func TestVerifierPassMovesOn(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	r.toReview("feature.txt")
	r.advance() // review: the job is queued
	var job jobsapi.Job
	if err := json.Unmarshal(r.verifierJob().Payload, &job); err != nil {
		t.Fatal(err)
	}
	if !job.ReadOnly || job.BaseSHA != r.task.HeadSha.String || r.task.State != "review" {
		t.Fatalf("job %+v, task in %s", job, r.task.State)
	}
	r.advance() // still running: nothing moves
	if r.task.State != "review" {
		t.Fatalf("waiting moved the task to %s", r.task.State)
	}
	r.verdict(`{"verdict":"pass","summary":"ok"}`)
	r.advance()
	if r.task.State != "preview" {
		t.Fatalf("after a pass the task is in %s", r.task.State)
	}
}

func TestVerifierReturnSendsFindingsToBuilder(t *testing.T) {
	db := pgtest.Start(t)
	r := newRigOnBook(t, db)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	r.toReview("feature.txt")
	r.advance()
	r.verdict(`{"verdict":"return","summary":"x","raise_blast_radius":{"level":"high","reason":"touches auth"},"findings":[{"rule_id":"new","category":"code","severity":"block","layer_caught":"L5","message":"the export drops the last row"}]}`)
	restore := r.rejectRaisedRadius(db)
	countFindings := func(want int) {
		t.Helper()
		found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
		if err != nil || len(found) != want {
			t.Fatalf("findings = %d, err = %v, want %d", len(found), err, want)
		}
	}
	countFindings(1)
	restore()
	r.advance()
	countFindings(1)
	if r.task.State != "building" {
		t.Fatalf("a return left the task in %s", r.task.State)
	}
	r.advance() // building: the next Builder job carries the finding
	if !strings.Contains(string(r.lastJob().Payload), "the export drops the last row") {
		t.Fatalf("state %s, payload %s", r.task.State, r.lastJob().Payload)
	}
}

func TestVerifierRaisedRadiusHoldsForAPerson(t *testing.T) {
	db := pgtest.Start(t)
	r := newRigOnBook(t, db)
	r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "solo")
	r.withVerifier()
	r.toReview("docs/guides/x.md")
	r.advance()
	r.verdict(`{"verdict":"pass","summary":"x","raise_blast_radius":{"level":"critical","reason":"touches auth"}}`)
	r.rejectRaisedRadius(db)()
	r.advance() // review -> preview
	r.advance() // preview -> acceptance -> human_review
	n, err := r.q.CountTaskEvents(t.Context(), bookdb.CountTaskEventsParams{TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: "review.raised"})
	if err != nil || n != 1 || r.task.State != "human_review" {
		t.Fatalf("raised events %d (%v), state %s", n, err, r.task.State)
	}
}

// rejectRaisedRadius checks that a lost risk write keeps the verdict available for retry.
func (r *rig) rejectRaisedRadius(db *pgtest.DB) func() {
	t := r.t
	t.Helper()
	ctx := t.Context()
	if _, err := db.Owner.ExecContext(ctx, `ALTER TABLE book.events
		ADD CONSTRAINT fixture_unavailable_radius CHECK (kind <> 'review.raised')`); err != nil {
		t.Fatal(err)
	}
	err := r.c.Advance(ctx, r.tenant, r.task)
	var storageErr *pgconn.PgError
	if !errors.As(err, &storageErr) || storageErr.ConstraintName != "fixture_unavailable_radius" {
		t.Fatalf("raised radius write returned %v, want the original storage error", err)
	}
	r.reload()
	if r.task.State != "review" || r.verifierJob().ConsumedAt.Valid {
		t.Fatalf("task moved or consumed its verdict despite a failed risk write: %s", r.task.State)
	}
	return func() {
		if _, err := db.Owner.ExecContext(ctx, "ALTER TABLE book.events DROP CONSTRAINT fixture_unavailable_radius"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVerifierEscalationStopsTheTask(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	r.toReview("feature.txt")
	r.advance()
	r.verdict(`{"verdict":"escalate","summary":"x","escalation_reason":"the spec contradicts itself"}`)
	r.advance()
	if r.task.State != "escalated" {
		t.Fatalf("an escalation left the task in %s", r.task.State)
	}
}

// limitJob ends a job on a provider limit that has already cleared.
func (r *rig) limitJob(id int64) {
	r.t.Helper()
	if _, err := r.c.Pool.Exec(r.t.Context(), `UPDATE book.jobs SET state = 'failed', exit_reason = 'rate_limit', finished_at = now() - interval '12 hours' WHERE id = $1`, id); err != nil {
		r.t.Fatal(err)
	}
}

// TestVerifierLimitsDoNotSpendTheReturnBudget: two jobs that hit a limit gave no verdict, so
// the first real "return" still goes back to Builder instead of escalating (MaxReturns is 2).
func TestVerifierLimitsDoNotSpendTheReturnBudget(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	r.toReview("feature.txt")
	r.advance() // queues the first job
	for range 2 {
		r.limitJob(r.verifierJob().ID)
		r.advance() // closes the job
		r.advance() // queues the next one
	}
	r.verdict(`{"verdict":"return","summary":"x","findings":[{"rule_id":"new","category":"code","severity":"block","layer_caught":"L5","message":"the export drops the last row"}]}`)
	r.advance()
	if r.task.State != "building" {
		t.Fatalf("limits spent the budget: the task is in %s", r.task.State)
	}
}
