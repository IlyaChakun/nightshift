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

// The worker reads mirrors that another user owns. Git's test switch makes every repository
// look that way. Newer git versions hand -c to the reading process, so the checkout alone
// passes there even without the fix: the recorded arguments pin the option the worker's older
// git needs. The directory name checks the quoting.
func TestCheckoutOfAMirrorOwnedByAnotherUser(t *testing.T) {
	t.Parallel()
	up, base := origin(t)
	g := &gitops.Git{Dir: filepath.Join(t.TempDir(), "it's mirrors")}
	if err := os.MkdirAll(g.Dir, 0o750); err != nil {
		t.Fatal(err)
	}
	mirror, err := g.EnsureMirror(t.Context(), "example/app", up, gitops.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	wrapper, args := filepath.Join(t.TempDir(), "git"), filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\ncase \"$*\" in *' clone '*) export GIT_TEST_ASSUME_DIFFERENT_OWNER=1\n" +
		"printf '%s\\n' \"$@\" >'" + args + "' ;; esac\nexec git \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil { //nolint:gosec // G306: the test runs this script
		t.Fatal(err)
	}
	g.Bin = wrapper
	if err := g.Checkout(t.Context(), mirror, filepath.Join(t.TempDir(), "task"), base); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(args) //nolint:gosec // G304: a path inside the test's temporary directory
	if err != nil {
		t.Fatal(err)
	}
	quoted := "'safe.directory=" + strings.ReplaceAll(mirror, "'", `'\''`) + "'"
	if want := "--upload-pack=git -c " + quoted + " upload-pack\n"; !strings.Contains(string(got), want) {
		t.Fatalf("the reading process is not told to trust the mirror: %s", got)
	}
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

// After Docker stopped mid-fetch every written ref of a mirror was an empty file, and the
// merge train retried on it forever.
func TestBrokenMirrorIsClonedAgain(t *testing.T) {
	t.Parallel()
	up, _ := origin(t)
	g := &gitops.Git{Dir: t.TempDir()}
	mirror, err := g.EnsureMirror(t.Context(), "example/app", up, gitops.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(mirror, "packed-refs")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	broken := filepath.Join(mirror, "refs", "heads", "main")
	if err := os.MkdirAll(filepath.Dir(broken), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.EnsureMirror(t.Context(), "example/app", up, gitops.Auth{}); err != nil {
		t.Fatalf("a broken mirror was not recovered: %v", err)
	}
	if _, err := g.Rev(t.Context(), mirror, "main"); err != nil {
		t.Errorf("main is not back: %v", err)
	}
}

// A code host publishes the commits of pull requests from forks under refs/pull. They must
// reach neither the mirror nor the checkout an agent reads.
func TestForeignRefsStayOutOfTheMirror(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{"new mirror": false, "mirror cloned with every ref": true}
	for name, legacy := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			up, base, forked := originWithFork(t)
			g := &gitops.Git{Dir: t.TempDir()}
			if legacy {
				oldMirror(t, g, up)
			}
			for range 2 { // the first call clones or repairs, the second fetches
				mirrorAndCheckout(t, g, up, base, forked)
			}
		})
	}
}

// mirrorAndCheckout refreshes the mirror, checks base out of it and fails when either holds
// anything but the branches and tags of the origin.
func mirrorAndCheckout(t *testing.T, g *gitops.Git, up, base string, forked []string) {
	t.Helper()
	mirror, err := g.EnsureMirror(t.Context(), "example/app", up, gitops.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "task")
	if err := g.Checkout(t.Context(), mirror, dest, base); err != nil {
		t.Fatal(err)
	}
	lacksObjects(t, []string{mirror, dest}, forked...)
	want := "refs/heads/main\nrefs/tags/v1"
	if got := sh(t, mirror, "for-each-ref", "--format=%(refname)"); got != want {
		t.Fatalf("refs %q, want %q", got, want)
	}
}

// A service that starts must clean old mirrors before the worker reads them, with or without
// the remote, and must not lose a branch that was never pushed.
func TestOldMirrorsAreConvertedWithoutTheRemote(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{"foreign refs": false, "foreign refs already deleted": true}
	for name, refsDeleted := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			up, base, forked := originWithFork(t)
			g := &gitops.Git{Dir: t.TempDir()}
			mirror := oldMirror(t, g, up)
			sh(t, mirror, "update-ref", "refs/heads/ns/1", base)
			if refsDeleted { // the objects stay behind
				sh(t, mirror, "update-ref", "-d", "refs/pull/1/head")
			}
			if err := g.ConvertMirrors(t.Context()); err != nil {
				t.Fatal(err)
			}
			lacksObjects(t, []string{mirror}, forked...)
			want := "refs/heads/main\nrefs/heads/ns/1\nrefs/tags/v1"
			if got := sh(t, mirror, "for-each-ref", "--format=%(refname)"); got != want {
				t.Fatalf("refs %q, want %q", got, want)
			}
		})
	}
}

func TestUnreachableRemoteKeepsAnOldMirror(t *testing.T) {
	t.Parallel()
	up, _, forked := originWithFork(t)
	g := &gitops.Git{Dir: t.TempDir()}
	mirror := oldMirror(t, g, up)
	if _, err := g.EnsureMirror(t.Context(), "example/app", filepath.Join(t.TempDir(), "gone"), gitops.Auth{}); err == nil {
		t.Fatal("fetch from a missing remote succeeded")
	}
	if _, err := g.Rev(t.Context(), mirror, "main"); err != nil {
		t.Errorf("the mirror is gone: %v", err)
	}
	lacksObjects(t, []string{mirror}, forked...)
}

// oldMirror clones the origin as a git mirror, with every ref.
func oldMirror(t *testing.T, g *gitops.Git, up string) string {
	t.Helper()
	mirror, err := g.MirrorPath("example/app")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(mirror), 0o750); err != nil {
		t.Fatal(err)
	}
	sh(t, g.Dir, "clone", "-q", "--mirror", up, mirror)
	return mirror
}

func TestHealthyMirrorIsFetchedNotClonedAgain(t *testing.T) {
	t.Parallel()
	up, _, _ := originWithFork(t)
	g := &gitops.Git{Dir: t.TempDir()}
	mirror, err := g.EnsureMirror(t.Context(), "example/app", up, gitops.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(mirror, "marker")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.EnsureMirror(t.Context(), "example/app", up, gitops.Auth{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the mirror was cloned again: %v", err)
	}
}

// originWithFork is an origin whose refs/pull holds a commit no branch reaches; forked names
// that commit and its new file.
func originWithFork(t *testing.T) (dir, base string, forked []string) {
	t.Helper()
	dir, base = origin(t)
	sh(t, dir, "tag", "v1")
	sh(t, dir, "checkout", "-q", "-b", "fork")
	commitFile(t, dir, "evil.txt", "from a fork\n")
	forked = []string{sh(t, dir, "rev-parse", "HEAD"), sh(t, dir, "rev-parse", "HEAD:evil.txt")}
	sh(t, dir, "update-ref", "refs/pull/1/head", forked[0])
	sh(t, dir, "checkout", "-q", "main")
	sh(t, dir, "branch", "-q", "-D", "fork")
	return dir, base, forked
}

// lacksObjects fails when a repository holds any of the objects, reachable or not.
func lacksObjects(t *testing.T, dirs []string, ids ...string) {
	t.Helper()
	for _, dir := range dirs {
		objects := sh(t, dir, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)")
		for _, id := range ids {
			if strings.Contains(objects, id) {
				t.Fatalf("%s holds the object %s", dir, id)
			}
		}
	}
}

// An unreachable remote says nothing about the mirror, which must be kept.
func TestUnreachableRemoteKeepsTheMirror(t *testing.T) {
	t.Parallel()
	g := &gitops.Git{Dir: t.TempDir()}
	up, _ := origin(t)
	mirror, err := g.EnsureMirror(t.Context(), "example/app", up, gitops.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.EnsureMirror(t.Context(), "example/app", filepath.Join(t.TempDir(), "gone"), gitops.Auth{}); err == nil {
		t.Fatal("fetch from a missing remote succeeded")
	}
	if left, _ := filepath.Glob(mirror + ".broken-*"); len(left) != 0 {
		t.Errorf("a healthy mirror was moved aside: %v", left)
	}
}
