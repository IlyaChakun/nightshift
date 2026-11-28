package dispatch_test

import (
	"fmt"
	"strings"
	"testing"

	deployfake "github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/control/mask"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
	"github.com/ilyachakun/nightshift/internal/customization/config"
)

// humanMergeRig is a solo task in human_review with a release, a chat thread and an outbox, so
// a card about an outside merge can be read back.
func humanMergeRig(t *testing.T, profile string) (*rig, *deployfake.Deployer) {
	t.Helper()
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, profile)
	d := deployfake.New()
	r.withRelease(d)
	r.c.Outbox = &outbox.Outbox{Pool: r.c.Pool, Mask: mask.NewMasker()}
	r.toReview("feature.txt")
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the task is in %s, want human_review", r.task.State)
	}
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET thread_ref = 'C1/1700000000.000100' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	r.reload()
	return r, d
}

// outsideCard is the text of the card about an outside merge, empty when none was queued.
func (r *rig) outsideCard() string {
	r.t.Helper()
	var payload string
	err := r.c.Pool.QueryRow(r.t.Context(), `SELECT payload::text FROM book.outbox WHERE idempotency_key = $1`, fmt.Sprintf("outside:%d:merged", r.task.ID)).Scan(&payload)
	if err != nil {
		return ""
	}
	return payload
}

// An approver who presses Merge on GitHub instead of Approve, on the head the factory verified,
// has approved it: the task is released like after an approval (live V4, 2026-11-28).
func TestAnApproversMergeOfTheVerifiedHeadIsReleased(t *testing.T) {
	r, d := humanMergeRig(t, "solo")
	r.twin.MergeAs("example/app", 1, "owner")
	r.advance()
	if r.task.State != "released" || d.Production() == "" {
		t.Fatalf("state %s, production %q; want released with a live revision", r.task.State, d.Production())
	}
	if card := r.outsideCard(); card != "" {
		t.Fatalf("a released task got an outside-merge card: %s", card)
	}
}

// A task already approved (ready) whose pull request an approver merged before the train did is
// released too; this is also where a crash between the two moves of a human merge resumes.
func TestAnApproversMergeOfAReadyTaskIsReleased(t *testing.T) {
	r, d := legacyPublication(t, "ready", "solo")
	r.twin.MergeAs("example/app", 1, "lead")
	r.advance()
	if r.task.State != "released" || d.Production() == "" {
		t.Fatalf("state %s, production %q; want released", r.task.State, d.Production())
	}
}

// Any other merge ends the task without a release and tells the owner why.
func TestAMergeThatIsNoApprovalEndsTheTaskWithoutARelease(t *testing.T) {
	for _, tc := range []struct {
		name, why string
		act       func(r *rig)
	}{
		{"merger unknown", "cannot approve", func(r *rig) { r.twin.Merge("example/app", 1) }},
		{"stranger", "cannot approve", func(r *rig) { r.twin.MergeAs("example/app", 1, "stranger") }},
		{"other head", "not the one the factory verified", func(r *rig) {
			gitIn(t, r.up, "checkout", "-q", "ns/1")
			gitIn(t, r.up, "commit", "-q", "--allow-empty", "-m", "someone else")
			gitIn(t, r.up, "checkout", "-q", "main")
			r.twin.MergeAs("example/app", 1, "owner")
		}},
		{"changes requested", "requests changes", func(r *rig) {
			r.twin.AddReview("example/app", 1, fake.Review{User: "lead", State: "changes_requested", CommitSHA: r.task.HeadSha.String})
			r.twin.MergeAs("example/app", 1, "owner")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, d := humanMergeRig(t, "solo")
			tc.act(r)
			r.advance()
			if r.task.State != "merged_by_team" || len(d.Calls()) != 0 {
				t.Fatalf("state %s, deploy calls %v; want merged_by_team and no release", r.task.State, d.Calls())
			}
			if card := r.outsideCard(); !strings.Contains(card, tc.why) {
				t.Fatalf("card %q does not say %q", card, tc.why)
			}
		})
	}
}

// A merge before the task reached a person (here during checks) is not an approval of a verified
// head: no release, and the owner reads why.
func TestAMergeBeforeVerificationIsNotReleased(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := deployfake.New()
	r.withRelease(d)
	r.c.Outbox = &outbox.Outbox{Pool: r.c.Pool, Mask: mask.NewMasker()}
	r.advance()
	r.finish(r.patchFor("feature.txt", "done\n"))
	r.advance()
	if r.task.State != "checks" {
		t.Fatalf("the task is in %s", r.task.State)
	}
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET thread_ref = 'C1/1700000000.000100' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	r.twin.MergeAs("example/app", 1, "owner")
	r.advance()
	if r.task.State != "merged_by_team" || len(d.Calls()) != 0 {
		t.Fatalf("state %s, deploy calls %v", r.task.State, d.Calls())
	}
	if card := r.outsideCard(); !strings.Contains(card, "had not passed") {
		t.Fatalf("card %q does not say why", card)
	}
}

// In the team profile the team owns the release: a merge at human review ends the task as before.
func TestATeamMergeAtHumanReviewIsNotReleasedByTheFactory(t *testing.T) {
	r, d := humanMergeRig(t, "team")
	r.twin.MergeAs("example/app", 1, "lead")
	r.advance()
	if r.task.State != "merged_by_team" || len(d.Calls()) != 0 {
		t.Fatalf("state %s, deploy calls %v", r.task.State, d.Calls())
	}
}
