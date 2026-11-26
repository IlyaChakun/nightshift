package dispatch_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/config"
)

func TestReviewRoundFollowsTheCurrentHead(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.toReview("feature.txt")
	r.advance()
	original := r.task.HeadSha.String

	gitIn(t, r.up, "checkout", "-q", "ns/1")
	gitIn(t, r.up, "commit", "-q", "--allow-empty", "-m", "update candidate")
	head := gitIn(t, r.up, "rev-parse", "HEAD")
	gitIn(t, r.up, "checkout", "-q", "main")
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: original})
	r.advance()
	if r.task.State != "human_review" || r.task.HeadSha.String != head {
		t.Fatalf("round = %s at %s, want review at %s", r.task.State, r.task.HeadSha.String, head)
	}
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: head})
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("after current approval: %s", r.task.State)
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
