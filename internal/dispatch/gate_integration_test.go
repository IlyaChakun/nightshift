package dispatch_test

import (
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/policy"
	"github.com/ilyachakun/nightshift/internal/train"
)

const gateRules = `
levels:
  low:
    paths: ["docs/guides/**"]
    max_diff_lines: 80
`

// toReview plays Builder and CI: the task ends up in `review` with one pushed patch.
func (r *rig) toReview(path string) {
	r.t.Helper()
	r.advance()
	r.finish(r.patchFor(path, "done\n"))
	r.advance()
	r.twin.SetChecks("example/app", r.task.HeadSha.String, fake.Check{Name: "test", Status: "completed", Conclusion: "success"})
	r.advance()
	if r.task.State != "review" {
		r.t.Fatalf("the task is in %s, want review", r.task.State)
	}
}

func (r *rig) withGate(review config.Review, governance string) {
	r.t.Helper()
	rules, err := policy.ParseRules([]byte(gateRules), config.NewValidator())
	if err != nil {
		r.t.Fatal(err)
	}
	r.c.Rules = rules
	r.c.Roll = func() float64 { return 0.99 }
	r.c.Train = &train.Train{Journal: train.BookJournal{Q: r.q}, Git: r.c, Forge: r.c.Forge, Mover: r.c.Mover}
	r.twin.HeadOf = func(_, branch string) string { return gitIn(r.t, r.up, "rev-parse", "refs/heads/"+branch) }
	r.tenant.Review = review
	r.tenant.Governance = governance
	r.tenant.People.Owner.GitHub = "owner"
	r.tenant.People.Approvers = []string{"lead"}
}

func TestSoloHumanReviewThenTrainMerges(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.toReview("feature.txt")

	r.advance() // review -> preview -> acceptance -> human_review
	if r.task.State != "human_review" {
		t.Fatalf("review mode all left the task in %s", r.task.State)
	}
	head := r.task.HeadSha.String
	if r.twin.Status("example/app", head, "nightshift/human-review") != "pending" {
		t.Fatal("the pull request is not held")
	}
	if pr, _ := r.twin.PR("example/app", 1); len(pr.Reviewers) == 0 || !strings.Contains(strings.Join(pr.Labels, ","), "human-review") {
		t.Fatalf("reviewers and label: %+v", pr)
	}
	r.advance() // nobody has looked yet
	if r.task.State != "human_review" {
		t.Fatalf("waiting moved the task to %s", r.task.State)
	}
	// A stranger's approval does not count.
	r.twin.AddReview("example/app", 1, fake.Review{User: "stranger", State: "approved", CommitSHA: head})
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("a stranger's approval moved the task to %s", r.task.State)
	}
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: head})
	r.advance()
	if r.task.State != "ready" || r.twin.Status("example/app", head, "nightshift/human-review") != "success" {
		t.Fatalf("after approval: %s, status %q", r.task.State, r.twin.Status("example/app", head, "nightshift/human-review"))
	}
	r.advance() // ready -> merging -> released through the train
	if r.task.State != "released" {
		t.Fatalf("after the train: %s", r.task.State)
	}
	if pr, _ := r.twin.PR("example/app", 1); !pr.Merged || pr.Draft {
		t.Fatalf("pull request: %+v", pr)
	}
}

func TestLowBlastRadiusSkipsHumanReview(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "solo")
	r.toReview("docs/guides/x.md")
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("a low change is in %s", r.task.State)
	}
	n, err := r.q.CountTaskEvents(t.Context(), bookdb.CountTaskEventsParams{TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: "review.decided"})
	if err != nil || n != 1 {
		t.Fatalf("decision events: %d %v", n, err)
	}
}

func TestChangesRequestedGoBackToBuilder(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.toReview("feature.txt")
	r.advance()
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "changes_requested", CommitSHA: r.task.HeadSha.String})
	r.advance()
	if r.task.State != "building" || !strings.Contains(string(r.lastJob().Payload), "review.changes_requested") {
		t.Fatalf("state %s, payload %s", r.task.State, r.lastJob().Payload)
	}
}

func TestTeamHandsOffAndWatchesTheMerge(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "team")
	r.toReview("feature.txt")
	r.advance()
	r.twin.AddReview("example/app", 1, fake.Review{User: "lead", State: "approved", CommitSHA: r.task.HeadSha.String})
	r.advance()
	r.advance() // ready -> handed_off
	if r.task.State != "handed_off" {
		t.Fatalf("the team profile is in %s", r.task.State)
	}
	r.advance()
	if r.task.State != "handed_off" {
		t.Fatalf("waiting moved the task to %s", r.task.State)
	}
	r.twin.Merge("example/app", 1)
	r.advance()
	if r.task.State != "merged_by_team" {
		t.Fatalf("after the team merged: %s", r.task.State)
	}
}
