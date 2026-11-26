package dispatch_test

import (
	"testing"

	deployfake "github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/train"
)

func legacyPublication(t *testing.T, state, profile string) (*rig, *deployfake.Deployer) {
	t.Helper()
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, profile)
	d := deployfake.New()
	r.withRelease(d)
	r.toReview("feature.txt")
	if _, err := r.c.Pool.Exec(t.Context(), "UPDATE book.tasks SET state=$2 WHERE id=$1", r.task.ID, state); err != nil {
		t.Fatal(err)
	}
	r.reload()
	return r, d
}

func TestResumedPublicationRequiresReviewEvidence(t *testing.T) {
	for _, tc := range []struct{ state, profile, approvedState string }{
		{"ready", "team", "handed_off"},
		{"merging", "solo", "released"},
	} {
		t.Run(tc.state, func(t *testing.T) {
			r, d := legacyPublication(t, tc.state, tc.profile)
			head := r.task.HeadSha.String
			r.twin.AddReview("example/app", 1, fake.Review{User: "stranger", State: "approved", CommitSHA: head})
			r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: "old"})
			r.advance()
			r.assertHeld("human_review", d)
			r.advance()
			r.assertHeld("human_review", d)
			if got := r.twin.Status("example/app", head, "nightshift/human-review"); got != "pending" {
				t.Fatalf("review status = %q, want pending", got)
			}
			r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: head})
			r.advance()
			r.advance()
			if r.task.State != tc.approvedState {
				t.Fatalf("state = %s after approval, want %s", r.task.State, tc.approvedState)
			}
		})
	}
}

func TestResumedMergeFinishesAnAlreadyMergedPR(t *testing.T) {
	r, d := legacyPublication(t, "merging", "solo")
	if _, err := r.c.Train.Journal.Start(t.Context(), r.task.Tenant, r.task.ID, train.StepMerge); err != nil {
		t.Fatal(err)
	}
	r.twin.Merge("example/app", 1)
	r.advance()
	if r.task.State != "released" || d.Production() == "" {
		t.Fatalf("state = %s, production = %q", r.task.State, d.Production())
	}
	steps, err := r.q.ListTrainSteps(t.Context(), r.task.ID)
	if err != nil || len(steps) != 1 || steps[0].Step != train.StepMerge || steps[0].State != "done" {
		t.Fatalf("train steps = %+v, err = %v, want a completed merge only", steps, err)
	}
}

func TestResumedMergeHoldsAnOldApprovalWithoutEvidence(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := deployfake.New()
	r.withRelease(d)
	r.toReview("feature.txt")
	r.advance() // Older versions could approve via SLA without a passing Verifier.
	for _, event := range []flow.Event{flow.Approve, flow.Merge} {
		if _, err := r.c.Mover.Apply(t.Context(), flow.Move{TaskID: r.task.ID, Event: event, Actor: "system", Profile: flow.Solo}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.c.Train.Journal.Start(t.Context(), r.task.Tenant, r.task.ID, train.StepRebase); err != nil {
		t.Fatal(err)
	}
	old := r.task.HeadSha.String
	r.twin.SetStatus("example/app", old, "nightshift/human-review", "success")
	gitIn(t, r.up, "commit", "-q", "--allow-empty", "-m", "advance main")
	rebased, err := r.c.Rebase(t.Context(), r.task.Repo, "ns/1")
	if err != nil || rebased.Conflict || rebased.SHA == old {
		t.Fatalf("rebase = %+v, err = %v, want a new head", rebased, err)
	}
	r.reload()
	// A push that survived a crash before the journal write cannot vouch for this head yet.
	if err := r.c.Advance(t.Context(), r.tenant, r.task); err == nil {
		t.Fatal("the missing rebase journal did not block publication")
	}
	r.reload()
	r.assertHeld("merging", d)
	if err := r.c.Train.Journal.Finish(t.Context(), r.task.ID, train.StepRebase, rebased.SHA); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.advance()
	r.assertHeld("human_review", d)
	steps, err := r.q.ListTrainSteps(t.Context(), r.task.ID)
	if err != nil || len(steps) != 0 {
		t.Fatalf("train steps = %+v, err = %v, want empty journal", steps, err)
	}
	if got := r.twin.Status("example/app", rebased.SHA, "nightshift/human-review"); got != "pending" || r.task.HeadSha.String != rebased.SHA {
		t.Fatalf("review status = %q, want pending", got)
	}
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: old})
	r.advance()
	r.assertHeld("human_review", d)
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: rebased.SHA})
	r.twin.SetChecks("example/app", rebased.SHA, fake.Check{Name: "test", Status: "completed", Conclusion: "success"})
	r.advance()
	r.advance()
	if r.task.State != "released" || d.Production() == "" {
		t.Fatalf("state = %s, production = %q", r.task.State, d.Production())
	}
}

func TestResumedPublicationHonorsAReviewReversal(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := deployfake.New()
	r.withRelease(d)
	r.withVerifier()
	r.toReview("feature.txt")
	r.advance()
	r.verdict(`{"verdict":"pass","summary":"ok"}`)
	r.advance()
	r.advance()
	head := r.task.HeadSha.String
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: head})
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("state = %s after approval, want ready", r.task.State)
	}
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "changes_requested", CommitSHA: head})
	r.advance()
	r.assertHeld("human_review", d)
}

// A rebase the train cannot vouch for drops the approval: the task waits for a person at the
// rebased head and merges only after that head is approved (design section 11).
func TestARebaseThatCannotVouchForThePatchNeedsAFreshApproval(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := deployfake.New()
	r.withRelease(d)
	r.toReview("feature.txt")
	r.advance()
	r.assertHeld("human_review", d)
	old := r.task.HeadSha.String
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: old})
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("state = %s after approval, want ready", r.task.State)
	}
	// A rebase started before a crash: its push may have landed, so it cannot vouch for the patch.
	if _, err := r.c.Mover.Apply(t.Context(), flow.Move{TaskID: r.task.ID, Event: flow.Merge, Actor: "system", Profile: flow.Solo}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.c.Train.Journal.Start(t.Context(), r.task.Tenant, r.task.ID, train.StepRebase); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "commit", "-q", "--allow-empty", "-m", "advance main")
	r.reload()
	r.advance()
	r.assertHeld("human_review", d)
	rebased := r.task.HeadSha.String
	if rebased == old || r.twin.Status("example/app", rebased, "nightshift/human-review") != "pending" {
		t.Fatalf("head %s (old %s), review status %q", rebased, old, r.twin.Status("example/app", rebased, "nightshift/human-review"))
	}
	if steps, err := r.q.ListTrainSteps(t.Context(), r.task.ID); err != nil || len(steps) != 0 {
		t.Fatalf("train steps = %+v, err = %v, want empty journal", steps, err)
	}
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: rebased})
	r.twin.SetChecks("example/app", rebased, fake.Check{Name: "test", Status: "completed", Conclusion: "success"})
	r.advance()
	r.advance()
	if r.task.State != "released" || d.Production() == "" {
		t.Fatalf("state = %s, production = %q", r.task.State, d.Production())
	}
}
