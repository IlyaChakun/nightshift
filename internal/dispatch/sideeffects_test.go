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

// repeatSafe lists every call by which the conductor and the merge train change something
// outside the book, with the reason a second call after a crash does no harm (plan R2, ADR
// 0018: every system we read runs a step at least once). A new call fails the test until
// someone adds it here with a reason, so the question is asked once per side effect.
var repeatSafe = map[string]string{
	"build.go Forge.OpenPR":          "a refusal returns the open pull request of the branch (TestOpenPRTwiceReturnsTheSamePullRequest)",
	"gate.go Forge.MarkReady":        "a pull request that is not a draft is left alone (TestHandOverFlow)",
	"gate.go Forge.AddLabels":        "GitHub adds a label that is already there without a change",
	"gate.go Forge.RequestReviewers": "GitHub ignores a reviewer that is already requested",
	"gate.go Forge.SetStatus":        "a commit status of the same context replaces the earlier one",
	"train.go Forge.Merge":           "a merge that reached GitHub before a crash shows as merged on the next look (train.merge), and a moved head is refused",
	"auth.go Git.Push":               "pushing the same commit to the same branch is a no-op",
	"build.go Git.ApplyPatch":        "the commit is rebuilt on the recorded base SHA and the branch is reset to it",
	"gate.go Git.Rebase":             "a rebase onto the same base is a no-op, and the train journal keeps its result",
	"auth.go Git.EnsureMirror":       "a fetch into the mirror, under its own lock",
	"chat.go Outbox.Enqueue":         "the outbox key makes a second message a no-op",
	"chatcmd.go Outbox.Enqueue":      "the outbox key makes a second message a no-op",
	"lead.go Outbox.Enqueue":         "the outbox key makes a second message a no-op",
	"morning.go Outbox.Enqueue":      "the outbox key carries the day, so one digest per tenant and day",
	"train.go Git.Rebase":            "a rebase onto the same base is a no-op, and the journal keeps the rebased SHA",
	"gate.go Train.Run":              "the train journals each step and reconciles with GitHub after a restart",
}

// sideEffects are the receivers whose methods reach outside the book and the methods of each
// that change something. Reads (Rev, Show, Changes, PRState, CI, Reviews) are not listed.
var sideEffects = map[string][]string{
	"Forge":  {"OpenPR", "MarkReady", "Merge", "RequestReviewers", "AddLabels", "SetStatus"},
	"Git":    {"Push", "ApplyPatch", "Rebase", "EnsureMirror"},
	"Outbox": {"Enqueue"},
	"Train":  {"Run"},
}

func TestEverySideEffectIsListedAsRepeatSafe(t *testing.T) {
	t.Parallel()
	found := map[string]bool{}
	for _, dir := range []string{".", "../train"} {
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
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				method, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				recv, ok := method.X.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if slices.Contains(sideEffects[recv.Sel.Name], method.Sel.Name) {
					found[filepath.Base(f)+" "+recv.Sel.Name+"."+method.Sel.Name] = true
				}
				return true
			})
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
