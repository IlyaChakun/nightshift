package dispatch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// repeatSafe lists every call by which the conductor, the doors, the periodic jobs and the merge
// train change something outside the book, with the reason a second call after a crash does no
// harm (plan R2, ADR 0018: every system we read runs a step at least once). A new call fails the test until
// someone adds it here with a reason, so the question is asked once per side effect.
var repeatSafe = map[string]string{
	"build.go Forge.OpenPR":          "a refusal returns the open pull request of the branch (TestOpenPRTwiceReturnsTheSamePullRequest)",
	"gate.go Forge.MarkReady":        "a pull request that is not a draft is left alone (TestHandOverFlow)",
	"gate.go Forge.AddLabels":        "a label already on the pull request stays one (checked on the twin; GitHub's own behavior is unverified)",
	"gate.go Forge.RequestReviewers": "a reviewer already requested stays one (checked on the twin; GitHub's own behavior is unverified)",
	"gate.go Forge.SetStatus":        "a commit status of the same context replaces the earlier one",
	"train.go Forge.Merge":           "a merge that reached GitHub before a crash shows as merged on the next look (train.merge), and a moved head is refused",
	"auth.go Git.Push":               "pushing the same commit to the same branch is a no-op",
	"build.go Git.ApplyPatch":        "the commit is rebuilt on the recorded base SHA and the branch is reset to it",
	"gate.go Git.Rebase":             "a rebase onto the same base is a no-op, and the train journal keeps its result",
	"auth.go Git.EnsureMirror":       "a fetch into the mirror, under its own lock",
	"chat.go Outbox.Enqueue":         "the outbox key makes a second message a no-op",
	"chatcmd.go Outbox.Enqueue":      "an answer to a chat command: the key holds the time of the command, so a redelivery repeats it; without a time a repeat gives a second answer card, nothing else (commands change the book only)",
	"lead.go Outbox.Enqueue":         "the outbox key makes a second message a no-op",
	"morning.go Outbox.Enqueue":      "the outbox key carries the day, so one digest per tenant and day",
	"tracker.go Outbox.Enqueue":      "the outbox key holds the task and its version, so a state is queued once; the adapter's Upsert gives the same task the same issue (TestTrackerProjectionAndTheTwoThingsThatComeBack)",
	"train.go Git.Rebase":            "a rebase onto the same base is a no-op, and the journal keeps the rebased SHA",
	"revert.go Forge.OpenPR":         "a refusal returns the open pull request of the revert branch (TestOpenPRTwiceReturnsTheSamePullRequest)",
	"revert.go Forge.Merge":          "a revert merged before a crash shows as merged on the next look (rolledBack), and a moved head is refused",
	"revert.go Git.Revert":           "the revert is rebuilt on the base branch and the revert branch is reset to it",
	"gate.go Train.Run":              "the train journals each step and reconciles with GitHub after a restart",
}

// sideEffects are the receivers whose methods reach outside the book and the methods of each
// that change something. Reads (Rev, Show, Changes, PRState, CI, Reviews) are not listed.
var sideEffects = map[string][]string{
	"Forge":  {"OpenPR", "MarkReady", "Merge", "RequestReviewers", "AddLabels", "SetStatus"},
	"Git":    {"Push", "ApplyPatch", "Rebase", "EnsureMirror", "Revert"},
	"Outbox": {"Enqueue"},
	"Train":  {"Run"},
}

func TestEverySideEffectIsListedAsRepeatSafe(t *testing.T) {
	t.Parallel()
	found := map[string]bool{}
	for _, dir := range []string{".", "../train", "../periodic", "../doors", "../tracking", "../routing", "../leadreply"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			refuseHiddenCalls(t, f, file)
			collectSideEffects(filepath.Base(f), file, found)
		}
	}
	for key := range found {
		if reason, ok := repeatSafe[key]; !ok || reason == "" {
			t.Errorf("%s changes something outside the book and has no reason in repeatSafe: say why a second call after a crash is harmless, or make it so", key)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(repeatSafe)) {
		if !found[key] {
			t.Errorf("repeatSafe lists %q, which no code calls any more: remove it", key)
		}
	}
}

// sideEffectOf names the call `x.Forge.Merge(...)` as "Forge.Merge"; ok is false for any other node.
func sideEffectOf(sel *ast.SelectorExpr) (string, bool) {
	recv, ok := sel.X.(*ast.SelectorExpr)
	if !ok || !slices.Contains(sideEffects[recv.Sel.Name], sel.Sel.Name) {
		return "", false
	}
	return recv.Sel.Name + "." + sel.Sel.Name, true
}

func collectSideEffects(name string, file *ast.File, found map[string]bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if what, ok := sideEffectOf(sel); ok {
					found[name+" "+what] = true
				}
			}
		}
		return true
	})
}

// refuseHiddenCalls fails on a receiver kept in a variable and on a method used as a value:
// collectSideEffects sees only `x.Forge.Merge(...)`, so these would hide a call from it.
func refuseHiddenCalls(t *testing.T, f string, file *ast.File) {
	t.Helper()
	funs := map[ast.Node]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			funs[call.Fun] = true
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, rhs := range n.Rhs {
				if sel, ok := rhs.(*ast.SelectorExpr); ok && sideEffects[sel.Sel.Name] != nil {
					t.Errorf("%s keeps %s in a variable; call it as x.%s.Method(...) so the inventory sees it", f, sel.Sel.Name, sel.Sel.Name)
				}
			}
		case *ast.SelectorExpr:
			if what, ok := sideEffectOf(n); ok && !funs[n] {
				t.Errorf("%s uses %s as a value; call it directly so the inventory sees it", f, what)
			}
		}
		return true
	})
}
