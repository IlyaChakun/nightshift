package dispatch_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/train"
)

// T4 (design section 7, "Rollback"): after a failed smoke rolled production back, the factory
// reverts the change on main through its own pull request, merges it once its checks pass, and
// sends the task back to Builder with an incident finding and no pull request.
func TestARollbackRevertsTheChangeAndSendsTheTaskBack(t *testing.T) {
	r, _ := runRelease(t, true, &integration.SmokeResult{Detail: "the start page is blank"})
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
	r, _ := runRelease(t, true, &integration.SmokeResult{Detail: "the start page is blank"})
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
