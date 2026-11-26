package dispatch_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/train"
)

func TestReviewRoundRequiresTheRecordedHead(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.toReview("feature.txt")
	r.advance()
	original := r.task.HeadSha.String

	gitIn(t, r.up, "checkout", "-q", "ns/1")
	gitIn(t, r.up, "commit", "-q", "--allow-empty", "-m", "update candidate")
	gitIn(t, r.up, "checkout", "-q", "main")
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: original})
	r.advance()
	if r.task.State != "human_review" || r.task.HeadSha.String != original {
		t.Fatalf("unconfirmed round = %s at %s", r.task.State, r.task.HeadSha.String)
	}
}

func TestAutomaticPublicationRespectsAReviewObjection(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "solo")
	r.toReview("docs/guides/export.md")
	r.passReview()
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("after automated review: %s", r.task.State)
	}
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "changes_requested", CommitSHA: r.task.HeadSha.String})
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("after the review objection: %s", r.task.State)
	}
}

func TestResumedPublicationRespectsCandidateObjection(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "solo")
	r.toReview("docs/guides/change.md")
	r.passReview()
	r.advance()
	if _, err := r.c.Mover.Apply(t.Context(), flow.Move{TaskID: r.task.ID, Event: flow.Merge, Actor: "system", Profile: flow.Solo}); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "checkout", "-q", "ns/1")
	gitIn(t, r.up, "commit", "-q", "--allow-empty", "-m", "candidate revision")
	head := gitIn(t, r.up, "rev-parse", "HEAD")
	gitIn(t, r.up, "checkout", "-q", "main")
	j := r.c.Train.Journal
	if _, err := j.Start(t.Context(), r.task.Tenant, r.task.ID, train.StepRebase); err != nil {
		t.Fatal(err)
	}
	if err := j.Finish(t.Context(), r.task.ID, train.StepRebase, head); err != nil {
		t.Fatal(err)
	}
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "changes_requested", CommitSHA: head})
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: r.task.HeadSha.String})
	r.reload()
	r.advance()
	if r.task.State != "human_review" || r.task.HeadSha.String != head {
		t.Fatalf("after candidate objection: %s at %s", r.task.State, r.task.HeadSha.String)
	}
}

func TestReviewObjectionFollowsAnEquivalentCandidate(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.toReview("feature.txt")
	r.advance()
	original := r.task.HeadSha.String
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: original})
	r.advance()
	if _, err := r.c.Mover.Apply(t.Context(), flow.Move{TaskID: r.task.ID, Event: flow.Merge, Actor: "system", Profile: flow.Solo}); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "commit", "-q", "--allow-empty", "-m", "update base")
	rebased, err := r.c.Rebase(t.Context(), r.task.Repo, "ns/1", original)
	if err != nil || !rebased.Identical {
		t.Fatalf("equivalent rebase: %+v, %v", rebased, err)
	}
	j := r.c.Train.Journal
	if _, err := j.Start(t.Context(), r.task.Tenant, r.task.ID, train.StepRebase); err != nil {
		t.Fatal(err)
	}
	if err := j.Finish(t.Context(), r.task.ID, train.StepRebase, rebased.SHA); err != nil {
		t.Fatal(err)
	}
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "changes_requested", CommitSHA: original})
	r.twin.AddReview("example/app", 1, fake.Review{User: "lead", State: "approved", CommitSHA: rebased.SHA})
	r.reload()
	r.advance()
	r.advance()
	if r.task.State != "building" {
		t.Fatalf("after equivalent-head objection: %s, want building", r.task.State)
	}
}
