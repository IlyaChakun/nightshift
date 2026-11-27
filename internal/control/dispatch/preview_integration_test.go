package dispatch_test

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"

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
