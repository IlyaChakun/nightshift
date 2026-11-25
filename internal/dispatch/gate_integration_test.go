package dispatch_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
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
	r.tenant.People.Approvers = []string{"owner", "github:lead"}
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
	r.twin.AddReview("example/app", 1, fake.Review{
		User: "owner", State: "changes_requested", CommitSHA: r.task.HeadSha.String, Body: "Rename the file.",
	})
	r.advance()
	payload := string(r.lastJob().Payload)
	if r.task.State != "building" || !strings.Contains(payload, "review.changes_requested") || !strings.Contains(payload, "Rename the file.") {
		t.Fatalf("state %s, payload %s", r.task.State, r.lastJob().Payload)
	}
}

func TestAStrangersReviewCostsNoCallAndBlocksNothing(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.toReview("feature.txt")
	r.advance()
	r.twin.FailLineComments()
	head := r.task.HeadSha.String
	r.twin.AddReview("example/app", 1, fake.Review{User: "stranger", State: "changes_requested", CommitSHA: head})
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: head})
	r.advance()
	if r.task.State == "human_review" || r.twin.LineCommentCalls() != 0 {
		t.Fatalf("state %s after %d calls", r.task.State, r.twin.LineCommentCalls())
	}
}

func TestAStaleRequestForChangesDoesNotLoopTheTask(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.toReview("feature.txt")
	r.advance()
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "changes_requested", CommitSHA: "0000000-an-older-head"})
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("a review of an older commit moved the task to %s", r.task.State)
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

func TestRulesComeFromTheRepositoryAndTheTickerDrivesTasks(t *testing.T) {
	r := newRig(t)
	if err := os.MkdirAll(filepath.Join(r.up, ".nightshift"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.up, ".nightshift", "blast-radius.yaml"), []byte(gateRules), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "add", ".")
	gitIn(t, r.up, "commit", "-q", "-m", "rules")
	r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "solo")
	r.c.Rules, r.c.Validator = nil, config.NewValidator()
	r.tenant.Name = "example-solo"

	tk := &dispatch.Ticker{
		Pool: r.c.Pool, Tenants: map[string]config.Tenant{"example-solo": r.tenant},
		For: func(config.Tenant) *dispatch.Conductor { return r.c }, Every: time.Hour, Log: slog.New(slog.DiscardHandler),
	}
	tk.Tick(t.Context()) // queued -> building, job queued
	r.reload()
	if r.task.State != "building" {
		t.Fatalf("after a tick: %s", r.task.State)
	}
	r.finish(r.patchFor("docs/guides/x.md", "done\n"))
	tk.Tick(t.Context())
	r.reload()
	r.twin.SetChecks("example/app", r.task.HeadSha.String, fake.Check{Name: "test", Status: "completed", Conclusion: "success"})
	tk.Tick(t.Context()) // checks -> review
	tk.Tick(t.Context()) // review -> ... -> ready, no human: the repository's rules call it low
	r.reload()
	if r.task.State != "ready" {
		t.Fatalf("a low change from the repository's rules is in %s", r.task.State)
	}
}

// T4: a rebase conflict at merge time sends the task back to checks and merges nothing.
func TestRebaseConflictAtMergeSendsTheTaskBackToChecks(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "solo")
	r.toReview("docs/guides/x.md")
	r.advance() // review -> ... -> ready (a low change needs no person)
	if r.task.State != "ready" {
		t.Fatalf("the task is in %s", r.task.State)
	}
	// Main moves on and touches the same file differently.
	if err := os.MkdirAll(filepath.Join(r.up, "docs", "guides"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.up, "docs", "guides", "x.md"), []byte("other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "add", ".")
	gitIn(t, r.up, "commit", "-q", "-m", "conflicting change on main")
	r.advance() // ready -> merging -> train: the rebase conflicts
	if r.task.State != "checks" {
		t.Fatalf("after a conflict the task is in %s", r.task.State)
	}
	if pr, _ := r.twin.PR("example/app", 1); pr.Merged {
		t.Fatal("a conflicting change was merged")
	}
}

// T4: cancel stops the task's jobs, and the conductor leaves a canceled task alone.
func TestCancelStopsJobsAndTheConductorLeavesTheTaskAlone(t *testing.T) {
	r := newRig(t)
	r.advance() // building, job queued
	if _, err := r.c.Mover.Apply(t.Context(), flow.Move{TaskID: r.task.ID, Event: flow.Cancel, Actor: "owner", Profile: flow.Solo}); err != nil {
		t.Fatal(err)
	}
	if got := r.lastJob().State; got != "canceled" {
		t.Fatalf("the job is %s", got)
	}
	tk := &dispatch.Ticker{
		Pool: r.c.Pool, Tenants: map[string]config.Tenant{"example-solo": {Name: "example-solo"}},
		For: func(config.Tenant) *dispatch.Conductor { return r.c }, Every: time.Hour, Log: slog.New(slog.DiscardHandler),
	}
	tk.Tick(t.Context())
	r.reload()
	if r.task.State != "canceled" {
		t.Fatalf("the task is in %s", r.task.State)
	}
	jobs, err := r.q.ListTaskJobs(t.Context(), bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "builder"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs after cancel: %d %v", len(jobs), err)
	}
}
