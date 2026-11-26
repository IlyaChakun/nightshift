package gitops_test

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/gitops"
)

// A squash merge and a merge commit are both undone on the base branch, and the revert lands on
// the named branch of the mirror only.
func TestRevertUndoesAMergedChange(t *testing.T) {
	t.Parallel()
	for _, merge := range []bool{false, true} {
		up, _ := origin(t)
		if merge {
			sh(t, up, "checkout", "-q", "-b", "feature")
			commitFile(t, up, "a.txt", "two\n")
			sh(t, up, "checkout", "-q", "main")
			commitFile(t, up, "b.txt", "other\n")
			sh(t, up, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
		} else {
			commitFile(t, up, "a.txt", "two\n")
		}
		bad := sh(t, up, "rev-parse", "HEAD")
		g := &gitops.Git{Dir: t.TempDir()}
		ctx := t.Context()
		mirror, err := g.EnsureMirror(ctx, "example/app", up, gitops.Auth{})
		if err != nil {
			t.Fatal(err)
		}
		head, err := g.Revert(ctx, mirror, "ns/7-revert", bad, bad, gitops.Identity{Name: "nightshift", Email: "ns@example.test"})
		if err != nil {
			t.Fatalf("merge=%v: %v", merge, err)
		}
		if got, ok, err := g.Show(ctx, mirror, head, "a.txt"); err != nil || !ok || strings.TrimSpace(got) != "one" {
			t.Errorf("merge=%v: a.txt after the revert = %q, %v, %v; want the content before the change", merge, got, ok, err)
		}
		if merge {
			if got, ok, _ := g.Show(ctx, mirror, head, "b.txt"); !ok || strings.TrimSpace(got) != "other" {
				t.Errorf("the revert of a merge also undid the base branch's own change: %q", got)
			}
		}
		if rev, err := g.Rev(ctx, mirror, "ns/7-revert"); err != nil || strings.TrimSpace(rev) != head {
			t.Errorf("branch = %q, %v; want %s", rev, err, head)
		}
	}
}

func TestRevertRefusesForeignBranches(t *testing.T) {
	t.Parallel()
	g := &gitops.Git{Dir: t.TempDir()}
	sha := strings.Repeat("a", 40)
	if _, err := g.Revert(t.Context(), t.TempDir(), "main", sha, sha, gitops.Identity{}); err == nil {
		t.Error("a revert onto main was accepted")
	}
}
