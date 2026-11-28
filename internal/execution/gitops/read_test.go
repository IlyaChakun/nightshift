package gitops_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/gitops"
)

// The deployer runs as root and reads mirrors that nightshift (uid 10001) owns. git refuses a
// repository owned by another user unless its path is trusted, so every read of a mirror must
// trust it, as Checkout does (V2 live run, 2026-11-28: "detected dubious ownership").
func TestReadsTrustAMirrorOwnedByAnotherUser(t *testing.T) {
	dir := t.TempDir()
	mirror := filepath.Join(dir, "owner", "repo.git")
	work := filepath.Join(dir, "work")
	gitIn := func(d string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // G204: fixed git commands of the test
		cmd.Dir = d
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, //nolint:forbidigo // git must be found
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid"}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.MkdirAll(work, 0o750); err != nil {
		t.Fatal(err)
	}
	gitIn(work, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "deploy.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(work, "add", "deploy.yaml")
	gitIn(work, "commit", "-q", "-m", "init")
	gitIn(dir, "clone", "-q", "--bare", work, mirror)
	sha := gitIn(work, "rev-parse", "HEAD")
	// git's own test switch makes every repository look owned by another user, so the check runs
	// as any user, in CI too; git runs through a wrapper that sets it.
	wrapper := filepath.Join(dir, "git-other-owner")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nGIT_TEST_ASSUME_DIFFERENT_OWNER=1 exec git \"$@\"\n"), 0o700); err != nil { //nolint:gosec // G306: a test script that has to be executable
		t.Fatal(err)
	}
	if _, err := (&gitops.Git{Bin: wrapper, Dir: dir}).Run(t.Context(), mirror, nil, nil, "rev-parse", "--git-dir"); err == nil || !strings.Contains(err.Error(), "dubious ownership") {
		t.Fatalf("the wrapper does not make git see another owner: %v", err)
	}
	g := &gitops.Git{Bin: wrapper, Dir: dir}
	ctx := t.Context()
	if got, err := g.Rev(ctx, mirror, "main"); err != nil || got != sha {
		t.Errorf("Rev = %q, %v; want %s", got, err, sha)
	}
	if got, ok, err := g.Show(ctx, mirror, sha, "deploy.yaml"); err != nil || !ok || got != "services: {}" {
		t.Errorf("Show = %q, %v, %v", got, ok, err)
	}
	if got, err := g.ObjectKind(ctx, mirror, sha, "deploy.yaml"); err != nil || got != "blob" {
		t.Errorf("ObjectKind = %q, %v; want blob", got, err)
	}
	if ok, err := g.IsRegularFile(ctx, mirror, sha, "deploy.yaml"); err != nil || !ok {
		t.Errorf("IsRegularFile = %v, %v; want true", ok, err)
	}
}
