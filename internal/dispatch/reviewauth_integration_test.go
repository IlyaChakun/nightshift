package dispatch_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/config"
)

// Anyone can review a pull request of a public repository. Only the owner and the approvers
// may move a task with a review (design section 16, "Forged commands").
func TestOutsidersReviewsChangeNothing(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.toReview("feature.txt")
	r.advance()
	head := r.task.HeadSha.String
	jobs := r.lastJob().ID

	for _, rv := range []fake.Review{
		{User: "stranger", State: "changes_requested"},
		{User: "stranger", State: "approved"},
		{User: "stranger", State: "dismissed"},
		{User: "", State: "approved"},
		{User: "owner-fan", State: "approved"},
		{User: "github:lead", State: "approved"},
	} {
		rv.CommitSHA = head
		r.twin.AddReview("example/app", 1, rv)
		r.advance()
		if r.task.State != "human_review" || r.lastJob().ID != jobs {
			t.Fatalf("%q %s moved the task to %s (last job %d, was %d)", rv.User, rv.State, r.task.State, r.lastJob().ID, jobs)
		}
		if got := r.twin.Status("example/app", head, "nightshift/human-review"); got != "pending" {
			t.Fatalf("%q %s set the status to %q", rv.User, rv.State, got)
		}
	}

	// An outsider who objects afterwards cannot undo an approver's decision.
	r.twin.AddReview("example/app", 1, fake.Review{User: "lead", State: "approved", CommitSHA: head})
	r.twin.AddReview("example/app", 1, fake.Review{User: "stranger", State: "changes_requested", CommitSHA: head})
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("after an approver and then a stranger: %s", r.task.State)
	}
}
