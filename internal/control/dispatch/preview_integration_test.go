package dispatch_test

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	deployfake "github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/control/deploy"
	"github.com/ilyachakun/nightshift/internal/control/train"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
)

// withPreview gives the tenant a preview capability on the deploy twin.
func (r *rig) withPreview() *deployfake.Deployer {
	r.t.Helper()
	d := deployfake.New()
	run := &deploy.Runner{Journal: deploy.BookJournal{Q: r.q}, Deployer: d}
	r.c.Deploys = func(context.Context, config.Tenant, bookdb.BookTask) (*deploy.Runner, bool, error) {
		return run, true, nil
	}
	return d
}

func TestAHealthyPreviewLetsTheTaskThrough(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")

	r.advance() // review -> preview (up, smoke) -> acceptance -> human_review
	if r.task.State != "human_review" {
		t.Fatalf("the task is in %s, want human_review", r.task.State)
	}
	env := deployapi.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	if st, _ := d.Status(t.Context(), env); st.State != deployapi.EnvHealthy || string(st.Revision) != r.task.HeadSha.String {
		t.Errorf("preview status = %+v, want healthy at the head", st)
	}
}

func TestABrokenPreviewSendsTheTaskBackToBuilderWithAFinding(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")
	env := deployapi.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	d.SetSmoke(env, deployapi.SmokeResult{Detail: "the start page is blank"})

	r.advance()
	if r.task.State != "building" {
		t.Fatalf("the task is in %s, want building", r.task.State)
	}
	found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
	if err != nil || len(found) != 1 || found[0].RuleID != "preview.smoke_failed" || !strings.Contains(found[0].Message, "blank") {
		t.Fatalf("findings = %+v, %v", found, err)
	}
	if st, _ := d.Status(t.Context(), env); st.State != deployapi.EnvAbsent {
		t.Errorf("the broken preview was not torn down: %+v", st)
	}
}

// A teardown that fails after the finding was stored must not store the finding again on the retry.
func TestABrokenPreviewRetryDoesNotRepeatTheFinding(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")
	env := deployapi.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	d.SetSmoke(env, deployapi.SmokeResult{Detail: "blank"})
	d.FailNext("down", errors.New("engine down"))

	if err := r.c.Advance(t.Context(), r.tenant, r.task); err == nil {
		t.Fatal("the failed teardown was not reported")
	}
	r.reload()
	r.advance()
	found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
	if err != nil || len(found) != 1 {
		t.Fatalf("findings = %d, %v; want exactly one", len(found), err)
	}
}

// A deployer that keeps failing must not be retried every tick forever (V2 live run,
// 2026-11-28: 26 failures, the task stayed in preview): the head gets a bounded number of
// tries, then the task escalates to a person.
func TestAPreviewThatKeepsFailingEscalates(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")
	for try := 1; ; try++ {
		d.FailNext("up", errors.New("composelocal: checkout: dubious ownership"))
		err := r.c.Advance(t.Context(), r.tenant, r.task)
		r.reload()
		if r.task.State == "escalated" {
			if try != 3 {
				t.Errorf("escalated after %d failures, want 3", try)
			}
			r.checkInfraEscalation(3)
			break
		}
		if err == nil || r.task.State != "preview" {
			t.Fatalf("failure %d: state %s, error %v; want preview and the error", try, r.task.State, err)
		}
		if try > 10 {
			t.Fatal("the failing preview never escalated")
		}
	}
}

// checkInfraEscalation checks that the last move says infrastructure, not a spent budget, and
// that each failure's cause is in the book.
func (r *rig) checkInfraEscalation(want int) {
	r.t.Helper()
	events, err := r.q.ListTaskEvents(r.t.Context(), pgtype.Int8{Int64: r.task.ID, Valid: true})
	if err != nil {
		r.t.Fatal(err)
	}
	last, causes := "", 0
	for _, e := range events {
		switch e.Kind {
		case "task.moved":
			last = string(e.Payload)
		case "preview.failed":
			if strings.Contains(string(e.Payload), "dubious ownership") {
				causes++
			}
		}
	}
	if !strings.Contains(last, `"infra_failed"`) || causes != want {
		r.t.Errorf("last move %s, causes %d; want infra_failed and %d", last, causes, want)
	}
}

// hangingUp is a deployer whose Up blocks until the step's deadline, as a hung `docker compose up` does.
type hangingUp struct{ *deployfake.Deployer }

func (hangingUp) Up(ctx context.Context, _ deployapi.EnvRef, _ deployapi.Revision) (deployapi.Endpoint, error) {
	<-ctx.Done()
	return deployapi.Endpoint{}, ctx.Err()
}

// A deployer that hangs until the step timeout counts as a failure too, although the step's
// context has expired by the time it is recorded (review of the bounded retry, 2026-11-28).
func TestAPreviewThatHangsEscalates(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	run := &deploy.Runner{Journal: deploy.BookJournal{Q: r.q}, Deployer: hangingUp{deployfake.New()}}
	r.c.Deploys = func(context.Context, config.Tenant, bookdb.BookTask) (*deploy.Runner, bool, error) {
		return run, true, nil
	}
	r.toReview("feature.txt")
	for try := 1; try <= 3; try++ {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		_ = r.c.Advance(ctx, r.tenant, r.task)
		cancel()
		r.reload()
	}
	if r.task.State != "escalated" {
		t.Fatalf("the task is in %s after three hung previews, want escalated", r.task.State)
	}
}

// toReady takes a task through review and the owner's approval; the next tick runs the train.
func (r *rig) toReady() {
	r.t.Helper()
	r.toReview("feature.txt")
	r.advance() // human_review
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: r.task.HeadSha.String})
	r.advance()
	if r.task.State != "ready" {
		r.t.Fatalf("the task is in %s, want ready", r.task.State)
	}
}

// withRelease gives the train a release on the deploy twin; the production smoke is the twin's.
// The code host twin reports a made-up merge commit, so the release first makes the merge real
// in the upstream repository and records that commit, as a revert needs it there.
func (r *rig) withRelease(d *deployfake.Deployer) {
	r.t.Helper()
	run := &deploy.Runner{Journal: deploy.BookJournal{Q: r.q}, Deployer: d}
	r.c.Train.Release = func(ctx context.Context, job train.Job, _ string) (deploy.ReleaseResult, bool, error) {
		sha := r.squashMerge(job)
		res, err := run.Release(ctx, deploy.Job{Tenant: job.Tenant, TaskID: job.TaskID, Rev: deployapi.Revision(sha)})
		return res, err == nil, err
	}
}

// squashMerge squashes the task branch onto main in the upstream repository once and records the
// commit as the train's merge.
func (r *rig) squashMerge(job train.Job) string {
	r.t.Helper()
	if st, ok, err := (train.BookJournal{Q: r.q}).Peek(r.t.Context(), job.TaskID, train.StepMerge); err == nil && ok && st.SHA != "" {
		// A repeated release finds the squash it made before.
		if exec.CommandContext(r.t.Context(), "git", "-C", r.up, "cat-file", "-e", st.SHA+"^{commit}").Run() == nil { //nolint:gosec // test helper with fixed arguments
			return st.SHA
		}
	}
	gitIn(r.t, r.up, "merge", "-q", "--squash", job.Branch)
	gitIn(r.t, r.up, "commit", "-q", "-m", "squash "+job.Branch)
	sha := gitIn(r.t, r.up, "rev-parse", "HEAD")
	if _, err := r.c.Pool.Exec(r.t.Context(), `UPDATE book.train_steps SET sha = $2 WHERE task_id = $1 AND step = 'merge'`, job.TaskID, sha); err != nil {
		r.t.Fatal(err)
	}
	return sha
}

const earlierRev = "0000000000000000000000000000000000000001"

// runRelease takes a task to the merge train with a release on the twin and returns the twin.
func runRelease(t *testing.T, previous bool, smoke *deployapi.SmokeResult) (*rig, *deployfake.Deployer) {
	t.Helper()
	return runReleaseOn(t, previous, func(d *deployfake.Deployer) {
		if smoke != nil {
			d.SetSmoke(deployapi.EnvRef{Tenant: "example-solo"}, *smoke)
		}
	})
}

// runReleaseOn is runRelease with a setup of the twin, for a failure that is not a smoke result.
func runReleaseOn(t *testing.T, previous bool, setup func(*deployfake.Deployer)) (*rig, *deployfake.Deployer) {
	t.Helper()
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := deployfake.New()
	if previous {
		if _, err := d.Release(t.Context(), earlierRev); err != nil {
			t.Fatal(err)
		}
	}
	setup(d)
	r.withRelease(d)
	r.toReady()
	r.advance() // ready -> merging -> release -> smoke
	if pr, _ := r.twin.PR("example/app", 1); !pr.Merged {
		t.Fatalf("the pull request was not merged: %+v", pr)
	}
	return r, d
}

// T4: the merge train releases through the deploy twin; a failed smoke rolls back on its own.
func TestTheTrainReleasesThroughTheDeployer(t *testing.T) {
	r, d := runRelease(t, false, nil)
	if r.task.State != "released" || d.Production() == "" {
		t.Fatalf("task %s, production %q; want released with a live revision", r.task.State, d.Production())
	}
}

func TestAFailedSmokeRollsBackToTheEarlierRevision(t *testing.T) {
	r, d := runRelease(t, true, &deployapi.SmokeResult{Detail: "the start page is blank"})
	if r.task.State != "rolled_back" || d.Production() != earlierRev {
		t.Fatalf("task %s, production %q; want rolled_back and the earlier revision", r.task.State, d.Production())
	}
}

// A revision that starts and never becomes healthy fails the release itself, before any smoke:
// production goes back to the earlier revision instead of the train retrying every tick.
func TestARolloutThatNeverBecomesHealthyRollsBack(t *testing.T) {
	r, d := runReleaseOn(t, true, func(d *deployfake.Deployer) { d.FailRollout() })
	if r.task.State != "rolled_back" || d.Production() != earlierRev {
		t.Fatalf("task %s, production %q; want rolled_back and the earlier revision", r.task.State, d.Production())
	}
}

func TestAFailedFirstReleaseWaitsForAPerson(t *testing.T) {
	r, _ := runRelease(t, false, &deployapi.SmokeResult{Detail: "the start page is blank"})
	if r.task.State != "escalated" {
		t.Fatalf("task %s, want escalated: there is nothing to roll back to", r.task.State)
	}
}

// The gate asks the review policy on every tick while a task waits in preview or acceptance; the
// decision is recorded once per state, not every 15 s (V4 on the Mac, 2026-11-28).
func TestAWaitingTaskRecordsItsReviewDecisionOnce(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "solo")
	d := r.withPreview()
	r.toReview("docs/guides/x.md")
	r.passReview()
	for range 2 { // two failures: the third would escalate
		d.FailNext("up", errors.New("engine busy"))
		_ = r.c.Advance(t.Context(), r.tenant, r.task)
		r.reload()
	}
	if r.task.State != "preview" {
		t.Fatalf("the task is in %s, want preview", r.task.State)
	}
	n, err := r.q.CountTaskEvents(t.Context(), bookdb.CountTaskEventsParams{TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: "review.decided"})
	if err != nil || n != 1 {
		t.Fatalf("decision events after two waiting ticks: %d %v; want 1", n, err)
	}
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("the task is in %s after the preview came up, want ready", r.task.State)
	}
}
