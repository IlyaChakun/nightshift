package dispatch_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	deployfake "github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/train"
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
	env := integration.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	if st, _ := d.Status(t.Context(), env); st.State != integration.EnvHealthy || string(st.Revision) != r.task.HeadSha.String {
		t.Errorf("preview status = %+v, want healthy at the head", st)
	}
}

func TestABrokenPreviewSendsTheTaskBackToBuilderWithAFinding(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")
	env := integration.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	d.SetSmoke(env, integration.SmokeResult{Detail: "the start page is blank"})

	r.advance()
	if r.task.State != "building" {
		t.Fatalf("the task is in %s, want building", r.task.State)
	}
	found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
	if err != nil || len(found) != 1 || found[0].RuleID != "preview.smoke_failed" || !strings.Contains(found[0].Message, "blank") {
		t.Fatalf("findings = %+v, %v", found, err)
	}
	if st, _ := d.Status(t.Context(), env); st.State != integration.EnvAbsent {
		t.Errorf("the broken preview was not torn down: %+v", st)
	}
}

// A teardown that fails after the finding was stored must not store the finding again on the retry.
func TestABrokenPreviewRetryDoesNotRepeatTheFinding(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")
	env := integration.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	d.SetSmoke(env, integration.SmokeResult{Detail: "blank"})
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
func (r *rig) withRelease(d *deployfake.Deployer) {
	r.t.Helper()
	run := &deploy.Runner{Journal: deploy.BookJournal{Q: r.q}, Deployer: d}
	r.c.Train.Release = func(ctx context.Context, job train.Job, sha string) (deploy.ReleaseResult, bool, error) {
		res, err := run.Release(ctx, deploy.Job{Tenant: job.Tenant, TaskID: job.TaskID, Rev: integration.Revision(sha)})
		return res, err == nil, err
	}
}

// T4: the merge train releases through the deploy twin; a failed smoke rolls back on its own.
func TestTheTrainReleasesThroughTheDeployer(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		previous   bool // a revision is already live
		smokeFails bool
	}{
		{"healthy", "released", false, false},
		{"failed smoke with an earlier revision rolls back", "rolled_back", true, true},
		{"failed first release has nothing to return to", "escalated", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.withGate(config.Review{Mode: "all"}, "solo")
			d := deployfake.New()
			if tc.previous {
				if _, err := d.Release(t.Context(), "0000000000000000000000000000000000000001"); err != nil {
					t.Fatal(err)
				}
			}
			if tc.smokeFails {
				d.SetSmoke(integration.EnvRef{Tenant: "example-solo"}, integration.SmokeResult{Detail: "the start page is blank"})
			}
			r.withRelease(d)
			r.toReady()

			r.advance() // ready -> merging -> release -> smoke
			if r.task.State != tc.want {
				t.Fatalf("after the train: %s, want %s", r.task.State, tc.want)
			}
			if pr, _ := r.twin.PR("example/app", 1); !pr.Merged {
				t.Fatalf("the pull request was not merged: %+v", pr)
			}
			prod := d.Production()
			switch tc.want {
			case "released", "escalated":
				if prod == "" {
					t.Error("nothing was released")
				}
			case "rolled_back":
				if prod != "0000000000000000000000000000000000000001" {
					t.Errorf("production = %q, want the earlier revision again", prod)
				}
			}
		})
	}
}
