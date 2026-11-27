package dispatch_test

import (
	"testing"

	deployfake "github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/train"
	"github.com/ilyachakun/nightshift/internal/customization/config"
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
	rebased, err := r.c.Rebase(t.Context(), r.task.Repo, "ns/1", old)
	if err != nil || rebased.Conflict || rebased.SHA == old {
		t.Fatalf("rebase = %+v, err = %v, want a new head", rebased, err)
	}
	r.reload()
	// A push that survived a crash before the journal write cannot vouch for this head: a
	// person reviews the head the code host now has.
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

// A crash after the train pushed its rebase leaves a head no approval covers. Even when the
// old head's approval no longer counts, the task goes to a person at the pushed head instead
// of failing every tick in merging.
func TestAPushedRebaseWithoutJournalGoesToReviewAtTheHostHead(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := deployfake.New()
	r.withRelease(d)
	r.toReview("feature.txt")
	r.advance()
	old := r.task.HeadSha.String
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: old})
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("state = %s after approval, want ready", r.task.State)
	}
	if _, err := r.c.Mover.Apply(t.Context(), flow.Move{TaskID: r.task.ID, Event: flow.Merge, Actor: "system", Profile: flow.Solo}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.c.Train.Journal.Start(t.Context(), r.task.Tenant, r.task.ID, train.StepRebase); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "commit", "-q", "--allow-empty", "-m", "advance main")
	rebased, err := r.c.Rebase(t.Context(), r.task.Repo, "ns/1", old)
	if err != nil || rebased.Conflict || rebased.SHA == old {
		t.Fatalf("rebase = %+v, err = %v, want a new head", rebased, err)
	}
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "changes_requested", CommitSHA: old})
	r.reload()
	r.advance()
	r.assertHeld("human_review", d)
	if r.task.HeadSha.String != rebased.SHA || r.twin.Status("example/app", rebased.SHA, "nightshift/human-review") != "pending" {
		t.Fatalf("head %s, want the pushed head %s with a pending review", r.task.HeadSha.String, rebased.SHA)
	}
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: rebased.SHA})
	r.twin.SetChecks("example/app", rebased.SHA, fake.Check{Name: "test", Status: "completed", Conclusion: "success"})
	r.advance()
	r.advance()
	if r.task.State != "released" || d.Production() == "" {
		t.Fatalf("state = %s, production = %q after approving the pushed head", r.task.State, d.Production())
	}
}

// A head that someone else pushed after an unfinished rebase is not the train's rebase: it
// needs reconciliation, so the task stays in merging and nobody is asked to approve it.
func TestAnOutsidePushAfterAnUnfinishedRebaseIsNotOfferedForApproval(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := deployfake.New()
	r.withRelease(d)
	r.toReview("feature.txt")
	r.advance()
	old := r.task.HeadSha.String
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: old})
	r.advance()
	if _, err := r.c.Mover.Apply(t.Context(), flow.Move{TaskID: r.task.ID, Event: flow.Merge, Actor: "system", Profile: flow.Solo}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.c.Train.Journal.Start(t.Context(), r.task.Tenant, r.task.ID, train.StepRebase); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "checkout", "-q", "ns/1")
	gitIn(t, r.up, "commit", "-q", "--allow-empty", "-m", "someone else")
	gitIn(t, r.up, "checkout", "-q", "main")
	r.reload()
	if err := r.c.Advance(t.Context(), r.tenant, r.task); err == nil {
		t.Fatal("an outside head was offered for approval")
	}
	r.reload()
	r.assertHeld("merging", d)
	if r.task.HeadSha.String != old {
		t.Fatalf("head %s, want the recorded %s", r.task.HeadSha.String, old)
	}
}
