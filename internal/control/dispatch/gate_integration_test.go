package dispatch_test

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/control/doors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/assurance/policy"
	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/mask"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
	"github.com/ilyachakun/nightshift/internal/control/train"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
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

// A pull request merged or closed on the code host while a person was to review it ends the
// task instead of leaving it in human_review for good.
func TestAnOutsideMergeOrCloseEndsATaskWaitingForAPerson(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		act        func(r *rig)
	}{
		{"merged", "merged_by_team", func(r *rig) { r.twin.Merge("example/app", 1) }},
		{"closed", "canceled", func(r *rig) { r.twin.Close("example/app", 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.withGate(config.Review{Mode: "all"}, "solo")
			r.toReview("feature.txt")
			r.advance()
			if r.task.State != "human_review" {
				t.Fatalf("the task is in %s", r.task.State)
			}
			tc.act(r)
			r.advance()
			if r.task.State != tc.want {
				t.Fatalf("after an outside %s: %s, want %s", tc.name, r.task.State, tc.want)
			}
		})
	}
}

// An outside merge during checks (before any person looks) is followed too, and the owner is told.
// A pull request closed while Builder works ends the task and stops Builder's job.
func TestAnOutsideCloseDuringBuildingEndsTheTaskAndStopsItsJob(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.advance()
	r.finish(r.patchFor("feature.txt", "done\n"))
	r.advance() // the pull request exists, the task is in checks
	r.twin.SetChecks("example/app", r.task.HeadSha.String, fake.Check{Name: "test", Status: "completed", Conclusion: "failure"})
	r.advance() // red CI sends the task back to Builder: a new job is queued
	if r.task.State != "building" {
		t.Fatalf("the task is in %s", r.task.State)
	}
	live := func() (n int) {
		t.Helper()
		if err := r.c.Pool.QueryRow(t.Context(), `SELECT count(*) FROM book.jobs WHERE task_id = $1 AND state IN ('queued', 'running')`, r.task.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if live() == 0 {
		t.Fatal("Builder has no job to stop: the test proves nothing")
	}
	r.twin.Close("example/app", 1)
	r.advance()
	if r.task.State != "canceled" {
		t.Fatalf("after an outside close during building: %s", r.task.State)
	}
	if live := live(); live != 0 {
		t.Fatalf("%d jobs of a canceled task are still live", live)
	}
}

func TestAnOutsideMergeDuringChecksEndsTheTaskAndTellsTheOwner(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
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
	r.twin.Merge("example/app", 1)
	r.advance()
	if r.task.State != "merged_by_team" {
		t.Fatalf("after an outside merge during checks: %s", r.task.State)
	}
	var cards int
	if err := r.c.Pool.QueryRow(t.Context(), `SELECT count(*) FROM book.outbox WHERE idempotency_key = $1`, fmt.Sprintf("outside:%d:merged", r.task.ID)).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if cards != 1 {
		t.Fatalf("the owner was not told: %d cards", cards)
	}
}

// The owner can cancel a task nothing watches any more, from the terminal; a released task
// cannot be canceled (only a rollback undoes it).
func TestCancelDoorEndsAWaitingTaskAndRefusesAReleasedOne(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.toReview("feature.txt")
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the task is in %s", r.task.State)
	}
	ctx := t.Context()
	d := &doors.Doors{Pool: r.c.Pool, Mover: r.c.Mover}
	if err := d.Cancel(ctx, r.tenant, r.task.ID, "cli"); err != nil {
		t.Fatal(err)
	}
	if task, _ := r.q.GetTask(ctx, r.task.ID); task.State != "canceled" {
		t.Fatalf("after cancel: %s", task.State)
	}
	if err := d.Cancel(ctx, r.tenant, r.task.ID, "cli"); !errors.Is(err, doors.ErrDoor) {
		t.Fatalf("a canceled task canceled again: %v", err)
	}
	if err := d.Cancel(ctx, r.tenant, 999999, "cli"); !errors.Is(err, doors.ErrDoor) {
		t.Fatalf("an unknown task: %v", err)
	}
}

func TestLowBlastRadiusSkipsHumanReview(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "solo")
	r.toReview("docs/guides/x.md")
	r.passReview()
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("a low change is in %s", r.task.State)
	}
	n, err := r.q.CountTaskEvents(t.Context(), bookdb.CountTaskEventsParams{TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: "review.decided"})
	if err != nil || n != 1 {
		t.Fatalf("decision events: %d %v", n, err)
	}
}

// A project that calls its scripts low cannot hide the one its verify command runs: the change
// is critical and waits for a person, while another script stays low.
func TestAScriptThatACommandRunsIsHeldWhateverTheRulesSay(t *testing.T) {
	for name, tc := range map[string]struct{ path, want string }{
		"the script of verify": {"tools/verify.sh", "human_review"},
		"an unrelated script":  {"tools/format.sh", "ready"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			if err := os.MkdirAll(filepath.Join(r.up, ".nightshift"), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(r.up, ".nightshift", "project.yaml"), []byte("verify: sh tools/verify.sh\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			gitIn(t, r.up, "add", ".")
			gitIn(t, r.up, "commit", "-q", "-m", "project")
			r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "solo")
			loose, err := policy.ParseRules([]byte("levels:\n  low:\n    paths: [\"tools/**\"]\n    max_diff_lines: 80\n"), config.NewValidator())
			if err != nil {
				t.Fatal(err)
			}
			r.c.Rules = loose
			r.toReview(tc.path)
			r.passReview()
			r.advance()
			if r.task.State != tc.want {
				t.Fatalf("a change to %s is in %s, want %s", tc.path, r.task.State, tc.want)
			}
		})
	}
}

// A project file that cannot be read holds the change for a person: nothing is judged on it.
func TestAnUnreadableProjectFileHoldsTheChange(t *testing.T) {
	r := newRig(t)
	if err := os.MkdirAll(filepath.Join(r.up, ".nightshift"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.up, ".nightshift", "project.yaml"), []byte("capabilities: [a, b]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "add", ".")
	gitIn(t, r.up, "commit", "-q", "-m", "broken project file")
	r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "solo")
	r.toReview("docs/guides/x.md")
	r.passReview()
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("a low change with an unreadable project file is in %s, want human_review", r.task.State)
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
	r.twin.FailLineComments(0)
	head := r.task.HeadSha.String
	r.twin.AddReview("example/app", 1, fake.Review{User: "stranger", State: "changes_requested", CommitSHA: head})
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "approved", CommitSHA: head})
	r.advance()
	if r.task.State == "human_review" || r.twin.LineCommentCalls() != 0 {
		t.Fatalf("state %s after %d calls", r.task.State, r.twin.LineCommentCalls())
	}
}

func TestAFailedCommentReadStoresNoFindingTwice(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.toReview("feature.txt")
	r.advance()
	head := r.task.HeadSha.String
	for _, user := range []string{"owner", "lead"} {
		r.twin.AddReview("example/app", 1, fake.Review{User: user, State: "changes_requested", CommitSHA: head, Body: "Change it."})
	}
	r.twin.FailLineComments(1) // the review of "owner", read after the one of "lead"
	for range 2 {
		if err := r.c.Advance(t.Context(), r.tenant, r.task); err == nil {
			t.Fatal("a failed read of comments was swallowed")
		}
	}
	found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
	if err != nil || len(found) != 0 {
		t.Fatalf("findings after failed reads: %d, %v", len(found), err)
	}
	r.twin.FailLineComments()
	r.advance()
	if found, err = r.q.ListTaskFindings(t.Context(), r.task.ID); err != nil || len(found) != 2 {
		t.Fatalf("findings: %d, %v", len(found), err)
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
	r.reload()
	r.passReview()
	tk.Tick(t.Context()) // the repository's rules call the verified change low
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
	r.passReview()
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
