package dispatch_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/control/mask"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
	"github.com/ilyachakun/nightshift/internal/control/train"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
)

// T4 (design section 7, "Rollback"): after a failed smoke rolled production back, the factory
// reverts the change on main through its own pull request, merges it once its checks pass, and
// sends the task back to Builder with an incident finding and no pull request.
func TestARollbackRevertsTheChangeAndSendsTheTaskBack(t *testing.T) {
	r, _ := runRelease(t, true, &deployapi.SmokeResult{Detail: "the start page is blank"})
	if r.task.State != "rolled_back" {
		t.Fatalf("the task is in %s, want rolled_back", r.task.State)
	}
	r.advance() // the revert pull request is opened
	rev, ok := r.twin.PR("example/app", 2)
	if !ok || !strings.HasSuffix(rev.Head, "-revert") || rev.Draft {
		t.Fatalf("revert pull request = %+v, %v", rev, ok)
	}
	// The revert branch no longer has the task's file.
	if out, err := exec.CommandContext(t.Context(), "git", "-C", r.up, "cat-file", "-e", rev.Head+":feature.txt").CombinedOutput(); err == nil { //nolint:gosec // test helper with fixed arguments
		t.Fatalf("feature.txt is still on the revert branch: %s", out)
	}
	r.advance() // its checks have not run: nothing happens
	if pr, _ := r.twin.PR("example/app", 2); pr.Merged || r.task.State != "rolled_back" {
		t.Fatalf("the revert merged before its checks: %+v, task %s", pr, r.task.State)
	}
	head := gitIn(t, r.up, "rev-parse", rev.Head) // the head the code host reports for the branch
	r.twin.SetChecks("example/app", head, fake.Check{Name: "test", Status: "completed", Conclusion: "success"})
	r.advance() // merged out of turn
	if pr, _ := r.twin.PR("example/app", 2); !pr.Merged {
		t.Fatalf("the green revert was not merged: %+v", pr)
	}
	r.advance() // back to Builder
	if r.task.State != "building" || r.task.PrNumber.Valid {
		t.Fatalf("task %s with pull request %v; want building with none", r.task.State, r.task.PrNumber)
	}
	found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
	if err != nil || !hasRule(found, "release.rolled_back") {
		t.Fatalf("findings = %+v, %v; want the incident", found, err)
	}
	if _, ok, err := (train.BookJournal{Q: r.q}).Peek(t.Context(), r.task.ID, train.StepMerge); err != nil || ok {
		t.Errorf("the old merge is still in the train journal: %v, %v", ok, err)
	}
}

// Regression (MVP validation V5, task #1012): a rollback tells the owner in the task's thread
// once, with what failed, the revision live again and the revert pull request.
func TestARollbackTellsTheOwnerOnce(t *testing.T) {
	r, _ := runRelease(t, true, &deployapi.SmokeResult{Detail: "the start page is blank"})
	r.c.Outbox = &outbox.Outbox{Pool: r.c.Pool, Mask: mask.NewMasker()}
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET thread_ref = 'C1/1700000000.000100' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.advance() // the revert pull request is opened
	r.advance() // its checks have not run: the card goes out
	r.advance() // a later tick queues no second card
	cards := r.cardsKeyed("rolled-back:%")
	if len(cards) != 1 {
		t.Fatalf("the owner got %d rollback cards, want 1", len(cards))
	}
	text := cards[0].Text
	for _, want := range []string{"the start page is blank", earlierRev[:7], "/pull/2", "rolled back"} {
		if !strings.Contains(text, want) {
			t.Errorf("the rollback card %q lacks %q", text, want)
		}
	}
}

func hasRule(fs []bookdb.BookFindingState, rule string) bool {
	for _, f := range fs {
		if f.RuleID == rule {
			return true
		}
	}
	return false
}

// While a task waits for its revert, the train of its repository holds every other merge.
func TestARollbackFreezesTheTrain(t *testing.T) {
	r, _ := runRelease(t, true, &deployapi.SmokeResult{Detail: "the start page is blank"})
	other, err := r.q.CreateTask(t.Context(), bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Another"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET state = 'merging', pr_number = 9 WHERE id = $1`, other.ID); err != nil {
		t.Fatal(err)
	}
	other, err = r.q.GetTask(t.Context(), other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.c.Advance(t.Context(), config.Tenant{Name: "example-solo", Governance: "solo"}, other); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := (train.BookJournal{Q: r.q}).Peek(t.Context(), other.ID, train.StepRebase); err != nil || ok {
		t.Errorf("the train ran for another task while the revert was pending: %v, %v", ok, err)
	}
}

// rollBackAgain stands for a second release of the task that failed its smoke: a new commit on
// main is the train's merge and the task is in rolled_back again.
func rollBackAgain(t *testing.T, r *rig, name string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.up, name), []byte("fix\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "add", ".")
	gitIn(t, r.up, "commit", "-q", "-m", "squash the fix")
	sha := gitIn(t, r.up, "rev-parse", "HEAD")
	j := train.BookJournal{Q: r.q}
	if _, err := j.Start(t.Context(), "example-solo", r.task.ID, train.StepMerge); err != nil {
		t.Fatal(err)
	}
	if err := j.Finish(t.Context(), r.task.ID, train.StepMerge, sha); err != nil {
		t.Fatal(err)
	}
	r.setState("rolled_back")
	return sha
}

func (r *rig) setState(state string) {
	r.t.Helper()
	if _, err := r.c.Pool.Exec(r.t.Context(), `UPDATE book.tasks SET state = $2 WHERE id = $1`, r.task.ID, state); err != nil {
		r.t.Fatal(err)
	}
	task, err := r.q.GetTask(r.t.Context(), r.task.ID)
	if err != nil {
		r.t.Fatal(err)
	}
	r.task = task
}

// Regression (review of the rollback revert): a task rolled back a second time gets a revert of
// its new commit, not the merged revert of the first incident.
func TestASecondRollbackRevertsTheNewCommit(t *testing.T) {
	r, _ := runRelease(t, true, &deployapi.SmokeResult{Detail: "the start page is blank"})
	r.advance()
	rev, _ := r.twin.PR("example/app", 2)
	r.twin.SetChecks("example/app", gitIn(t, r.up, "rev-parse", rev.Head), fake.Check{Name: "test", Status: "completed", Conclusion: "success"})
	r.advance()
	r.advance()
	if r.task.State != "building" {
		t.Fatalf("after the first revert the task is in %s", r.task.State)
	}
	rollBackAgain(t, r, "fix.txt")
	r.advance()
	if r.task.State != "rolled_back" {
		t.Fatalf("the second incident ended at once: task in %s", r.task.State)
	}
	again, ok := r.twin.PR("example/app", 3)
	if !ok || again.Merged {
		t.Fatalf("no new revert pull request for the second incident: %+v, %v", again, ok)
	}
	if out, err := exec.CommandContext(t.Context(), "git", "-C", r.up, "cat-file", "-e", again.Head+":fix.txt").CombinedOutput(); err == nil { //nolint:gosec // test helper with fixed arguments
		t.Fatalf("fix.txt is still on the second revert branch: %s", out)
	}
	found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
	if err != nil || len(found) == 0 {
		t.Fatalf("findings = %+v, %v", found, err)
	}
}

// Regression (review of the rollback revert): a revert that conflicts with main waits and tells
// the owner once, and a revert by hand lets the task go on and the train run again.
func TestAStuckRevertTellsTheOwnerAndAHandRevertFreesTheTrain(t *testing.T) {
	r, _ := runRelease(t, true, &deployapi.SmokeResult{Detail: "the start page is blank"})
	r.c.Outbox = &outbox.Outbox{Pool: r.c.Pool, Mask: mask.NewMasker()}
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET thread_ref = 'C1/1700000000.000100' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	bad := gitIn(t, r.up, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(r.up, "feature.txt"), []byte("hotfix\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "commit", "-q", "-am", "hotfix on the same lines")
	r.advance()
	r.advance() // a second tick queues no second card
	if _, ok := r.twin.PR("example/app", 2); ok || r.task.State != "rolled_back" {
		t.Fatalf("a conflicting revert opened a pull request or moved the task (%s)", r.task.State)
	}
	var cards int
	if err := r.c.Pool.QueryRow(t.Context(), `SELECT count(*) FROM book.outbox WHERE idempotency_key = $1`, fmt.Sprintf("revert-stuck:%d:%s:conflict", r.task.ID, bad)).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if cards != 1 {
		t.Fatalf("the owner got %d cards about the conflict, want 1", cards)
	}
	gitIn(t, r.up, "rm", "-q", "feature.txt")
	gitIn(t, r.up, "commit", "-q", "-m", "revert by hand")
	r.advance()
	if r.task.State != "building" || r.task.PrNumber.Valid {
		t.Fatalf("after a hand revert: task %s with pull request %v; want building with none", r.task.State, r.task.PrNumber)
	}
	frozen, err := dispatch.TrainFrozen(t.Context(), r.c, r.task)
	if err != nil || frozen {
		t.Errorf("the train is still frozen: %v, %v", frozen, err)
	}
}
