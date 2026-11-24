package gitops_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/gitops"
)

func sh(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// origin makes a repository with one commit.
func origin(t *testing.T) (dir, sha string) {
	t.Helper()
	dir = t.TempDir()
	sh(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh(t, dir, "add", ".")
	sh(t, dir, "commit", "-q", "-m", "init")
	return dir, sh(t, dir, "rev-parse", "HEAD")
}

func TestMirrorCheckoutPatchPush(t *testing.T) {
	t.Parallel()
	up, base := origin(t)
	g := &gitops.Git{Dir: t.TempDir()}
	ctx := context.Background()

	mirror, err := g.EnsureMirror(ctx, "example/app", up, gitops.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	// A second call fetches instead of failing.
	if _, err := g.EnsureMirror(ctx, "example/app", up, gitops.Auth{}); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "task-1")
	if err := g.Checkout(ctx, mirror, dest, base); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "a.txt")); err != nil || string(b) != "one\n" {
		t.Fatalf("checkout: %q %v", b, err)
	}
	if err := os.WriteFile(filepath.Join(dest, "a.txt"), []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	patch := sh(t, dest, "diff") + "\n"

	sha, err := g.ApplyPatch(ctx, mirror, "ns/1", base, []byte(patch), gitops.Identity{Name: "nightshift", Email: "ns@example.test"}, "feat: two")
	if err != nil {
		t.Fatal(err)
	}
	if got := sh(t, mirror, "rev-parse", "refs/heads/ns/1"); got != sha {
		t.Fatalf("branch %s, commit %s", got, sha)
	}
	if err := g.Push(ctx, mirror, up, "ns/1", gitops.Auth{}); err != nil {
		t.Fatal(err)
	}
	if got := sh(t, up, "rev-parse", "refs/heads/ns/1"); got != sha {
		t.Fatalf("pushed %s, want %s", got, sha)
	}
}

func TestRejectsBadInput(t *testing.T) {
	t.Parallel()
	g := &gitops.Git{Dir: t.TempDir()}
	ctx := context.Background()
	for _, repo := range []string{"", "../x", "a/../b", "-flag", "a/b/c"} {
		if _, err := g.MirrorPath(repo); err == nil {
			t.Errorf("repo %q accepted", repo)
		}
	}
	if err := g.Checkout(ctx, "m", filepath.Join(t.TempDir(), "d"), "--upload-pack=x"); err == nil {
		t.Error("a bad sha was accepted")
	}
	if _, err := g.ApplyPatch(ctx, "m", "main", strings.Repeat("a", 40), nil, gitops.Identity{}, "x"); err == nil {
		t.Error("a branch outside ns/ was accepted")
	}
	if err := g.Push(ctx, "m", "r", "../x", gitops.Auth{}); err == nil {
		t.Error("a bad push branch was accepted")
	}
}
