package gitops_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/policy"
)

func sh(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // test helper with fixed arguments
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), //nolint:forbidigo // git must be found
		"HOME=/nonexistent", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test",
	}
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
	g := &gitops.Git{Dir: t.TempDir(), Guard: guard(t)}
	ctx := t.Context()

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
	b, err := os.ReadFile(filepath.Join(dest, "a.txt")) //nolint:gosec // a path inside the test temp dir
	if err != nil || string(b) != "one\n" {
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
	// A second round replaces the branch on the remote.
	if err := os.WriteFile(filepath.Join(dest, "a.txt"), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	patch = sh(t, dest, "diff") + "\n"
	sha2, err := g.ApplyPatch(ctx, mirror, "ns/1", base, []byte(patch), gitops.Identity{Name: "nightshift", Email: "ns@example.test"}, "feat: three")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Push(ctx, mirror, up, "ns/1", gitops.Auth{}); err != nil {
		t.Fatalf("second push: %v", err)
	}
	if got := sh(t, up, "rev-parse", "refs/heads/ns/1"); got != sha2 {
		t.Fatalf("pushed %s, want %s", got, sha2)
	}
}

func guard(t *testing.T) func([]string) error {
	t.Helper()
	c, err := policy.NewConstitution(nil)
	if err != nil {
		t.Fatal(err)
	}
	return c.Check
}

// TestPatchOnProtectedPathIsRejected covers a plain edit and a rename into a protected path;
// the branch must not exist afterwards.
func TestPatchOnProtectedPathIsRejected(t *testing.T) {
	t.Parallel()
	up, base := origin(t)
	g := &gitops.Git{Dir: t.TempDir(), Guard: guard(t)}
	ctx := t.Context()
	mirror, err := g.EnsureMirror(ctx, "example/app", up, gitops.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(dest string){
		"new workflow": func(dest string) {
			if err := os.MkdirAll(filepath.Join(dest, ".github", "workflows"), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dest, ".github", "workflows", "x.yml"), []byte("on: push\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"rename into AGENTS.md": func(dest string) { sh(t, dest, "mv", "a.txt", "AGENTS.md") },
	} {
		dest := filepath.Join(t.TempDir(), "task")
		if err := g.Checkout(ctx, mirror, dest, base); err != nil {
			t.Fatal(err)
		}
		change(dest)
		sh(t, dest, "add", "-A")
		patch := sh(t, dest, "diff", "--cached", "-M") + "\n"
		_, err := g.ApplyPatch(ctx, mirror, "ns/9", base, []byte(patch), gitops.Identity{Name: "nightshift", Email: "ns@example.test"}, "x")
		if !errors.Is(err, policy.ErrProtected) {
			t.Errorf("%s: %v", name, err)
		}
		if out, err := exec.CommandContext(ctx, "git", "-C", mirror, "rev-parse", "--verify", "-q", "refs/heads/ns/9").CombinedOutput(); err == nil { //nolint:gosec // test with a temp directory
			t.Errorf("%s: the branch exists: %s", name, out)
		}
	}
}

func TestPatchWithoutGuardIsRefused(t *testing.T) {
	t.Parallel()
	g := &gitops.Git{Dir: t.TempDir()}
	if _, err := g.ApplyPatch(t.Context(), "m", "ns/1", strings.Repeat("a", 40), []byte("x"), gitops.Identity{}, "x"); err == nil {
		t.Error("a patch was applied without a guard")
	}
}

func TestRejectsBadInput(t *testing.T) {
	t.Parallel()
	g := &gitops.Git{Dir: t.TempDir()}
	ctx := t.Context()
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

// commitFile commits a file in a working repository.
func commitFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	sh(t, dir, "add", ".")
	sh(t, dir, "commit", "-q", "-m", "change "+name)
}

func TestRebaseOnAMovedBase(t *testing.T) {
	t.Parallel()
	up, base := origin(t)
	g := &gitops.Git{Dir: t.TempDir(), Guard: guard(t)}
	ctx := t.Context()
	mirror, err := g.EnsureMirror(ctx, "example/app", up, gitops.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	// The task branch adds b.txt; then main moves on in another file.
	sh(t, up, "checkout", "-q", "-b", "ns/1")
	commitFile(t, up, "b.txt", "task\n")
	sh(t, up, "checkout", "-q", "main")
	commitFile(t, up, "c.txt", "other\n")
	if _, err := g.EnsureMirror(ctx, "example/app", up, gitops.Auth{}); err != nil {
		t.Fatal(err)
	}
	id := gitops.Identity{Name: "nightshift", Email: "ns@example.test"}

	got, err := g.Rebase(ctx, mirror, "ns/1", "main", id)
	if err != nil || got.Conflict || !got.Identical || got.SHA == "" {
		t.Fatalf("clean rebase: %+v %v", got, err)
	}
	if parent := sh(t, mirror, "rev-parse", "refs/heads/ns/1~1"); parent != sh(t, mirror, "rev-parse", "refs/heads/main") {
		t.Fatalf("the branch is not on top of main: %s (base %s)", parent, base)
	}

	// A conflicting change on main: the branch stays where it was.
	sh(t, up, "checkout", "-q", "ns/1")
	commitFile(t, up, "a.txt", "task edit\n")
	sh(t, up, "checkout", "-q", "main")
	commitFile(t, up, "a.txt", "main edit\n")
	if _, err := g.EnsureMirror(ctx, "example/app", up, gitops.Auth{}); err != nil {
		t.Fatal(err)
	}
	before := sh(t, mirror, "rev-parse", "refs/heads/ns/1")
	got, err = g.Rebase(ctx, mirror, "ns/1", "main", id)
	if err != nil || !got.Conflict || got.SHA != "" {
		t.Fatalf("conflict: %+v %v", got, err)
	}
	if sh(t, mirror, "rev-parse", "refs/heads/ns/1") != before {
		t.Fatal("a conflicting rebase moved the branch")
	}
	if _, err := g.Rebase(ctx, mirror, "main", "main", id); err == nil {
		t.Fatal("a branch outside ns/ was rebased")
	}
}

func TestChangesListsPathsAndLines(t *testing.T) {
	t.Parallel()
	up, _ := origin(t)
	g := &gitops.Git{Dir: t.TempDir(), Guard: guard(t)}
	ctx := t.Context()
	mirror, err := g.EnsureMirror(ctx, "example/app", up, gitops.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	sh(t, up, "checkout", "-q", "-b", "ns/2")
	sh(t, up, "mv", "a.txt", "b.txt")
	commitFile(t, up, "c.txt", "x\ny\n")
	if _, err := g.EnsureMirror(ctx, "example/app", up, gitops.Auth{}); err != nil {
		t.Fatal(err)
	}
	paths, lines, err := g.Changes(ctx, mirror, "main", "ns/2")
	if err != nil {
		t.Fatal(err)
	}
	// A rename counts as both paths, so a move into a protected path cannot hide.
	for _, want := range []string{"a.txt", "b.txt", "c.txt"} {
		if !slices.Contains(paths, want) {
			t.Errorf("paths %v lack %s", paths, want)
		}
	}
	if lines != 4 {
		t.Errorf("lines = %d, want 4 (one removed, one added, two added)", lines)
	}
	if _, _, err := g.Changes(ctx, mirror, "main", "../x"); err == nil {
		t.Error("a bad branch was accepted")
	}
}

func TestShowReadsAFileAtACommit(t *testing.T) {
	t.Parallel()
	up, base := origin(t)
	g := &gitops.Git{Dir: t.TempDir(), Guard: guard(t)}
	ctx := t.Context()
	mirror, err := g.EnsureMirror(ctx, "example/app", up, gitops.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	if body, ok, err := g.Show(ctx, mirror, base, "a.txt"); err != nil || !ok || body != "one" {
		t.Fatalf("a.txt: %q %v %v", body, ok, err)
	}
	if _, ok, err := g.Show(ctx, mirror, base, ".nightshift/project.yaml"); err != nil || ok {
		t.Fatalf("a missing file: %v %v", ok, err)
	}
	if _, _, err := g.Show(ctx, mirror, base, "../etc/passwd"); err == nil {
		t.Fatal("a path outside the repository was accepted")
	}
}
