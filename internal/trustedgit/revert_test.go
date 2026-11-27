package trustedgit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/gitops"
	"github.com/ilyachakun/nightshift/internal/trustedgit"
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
		g := &trustedgit.Git{Git: &gitops.Git{Dir: t.TempDir()}}
		ctx := t.Context()
		mirror, err := g.EnsureMirror(ctx, "example/app", up, trustedgit.Auth{})
		if err != nil {
			t.Fatal(err)
		}
		head, err := g.Revert(ctx, mirror, "ns/7-revert", bad, bad, trustedgit.Identity{Name: "nightshift", Email: "ns@example.test"})
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
	g := &trustedgit.Git{Git: &gitops.Git{Dir: t.TempDir()}}
	sha := strings.Repeat("a", 40)
	if _, err := g.Revert(t.Context(), t.TempDir(), "main", sha, sha, trustedgit.Identity{}); err == nil {
		t.Error("a revert onto main was accepted")
	}
}

// A revert that a retry cannot fix says why: a person resolves a conflict, a commit outside the
// base branch is the wrong one, and an empty revert means the change is gone already.
func TestRevertSaysWhyItCannotApply(t *testing.T) {
	t.Parallel()
	id := trustedgit.Identity{Name: "nightshift", Email: "ns@example.test"}
	cases := map[string]struct {
		after func(t *testing.T, up, bad string) (sha string)
		want  error
	}{
		"a later commit changed the same lines": {
			after: func(t *testing.T, up, bad string) string { commitFile(t, up, "a.txt", "three\n"); return bad },
			want:  trustedgit.ErrRevertConflict,
		},
		"a person reverted it already": {
			after: func(t *testing.T, up, bad string) string { sh(t, up, "revert", "--no-edit", bad); return bad },
			want:  trustedgit.ErrNothingToRevert,
		},
		"the commit is not on the base branch": {
			after: func(t *testing.T, up, _ string) string {
				sh(t, up, "checkout", "-q", "-b", "side")
				commitFile(t, up, "c.txt", "side\n")
				side := sh(t, up, "rev-parse", "HEAD")
				sh(t, up, "checkout", "-q", "main")
				return side
			},
			want: trustedgit.ErrNotOnBase,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			up, _ := origin(t)
			commitFile(t, up, "a.txt", "two\n")
			sha := tc.after(t, up, sh(t, up, "rev-parse", "HEAD"))
			base := sh(t, up, "rev-parse", "main")
			g := &trustedgit.Git{Git: &gitops.Git{Dir: t.TempDir()}}
			mirror, err := g.EnsureMirror(t.Context(), "example/app", up, trustedgit.Auth{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := g.Revert(t.Context(), mirror, "ns/7-revert", base, sha, id); !errors.Is(err, tc.want) {
				t.Errorf("err = %v; want %v", err, tc.want)
			}
		})
	}
}
