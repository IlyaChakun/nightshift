// Package gitops is the worker's half of Git: task checkouts of the mirrors, the diff that
// becomes a job's patch, and reads of a mirror at a commit. Writing mirrors, applying patches,
// pushing, rebasing and reverting are trusted operations in package trustedgit, which only
// nightshift runs. Git runs with a scrubbed environment and no hooks, so repository content
// cannot run code (design section 16).
package gitops

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var (
	shaRe  = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)?$`)
	baseRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	// pathRe is a repository path; dot directories such as .nightshift are fine, `..` is not.
	pathRe = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._/-]*$`)
)

// IsSHA reports a full commit id, the only form of a commit the Git operations take.
func IsSHA(s string) bool { return shaRe.MatchString(s) }

// IsBranch reports a branch name the Git operations take, such as a base branch.
func IsBranch(s string) bool { return baseRe.MatchString(s) }

// Git runs git commands under Dir, the root of the `repos` volume.
type Git struct {
	Dir string
	Bin string // defaults to "git"
	// UID and GID, when UID is set, run git as that user. The worker is root and must never
	// run git as root in a checkout the agent can write to: its config could run code.
	UID, GID uint32
	// MirrorGID, when UID is set, is a supplementary group of the clone alone: it lets the
	// worker read the mirrors nightshift keeps, which the agent itself must not browse.
	MirrorGID uint32
}

func (g *Git) bin() string {
	if g.Bin != "" {
		return g.Bin
	}
	return "git"
}

// run executes git in dir. Hooks, filters of the system and user config, and prompts are off.
func (g *Git) run(ctx context.Context, dir string, stdin []byte, extraEnv []string, args ...string) (string, error) {
	return g.runGroups(ctx, nil, dir, stdin, extraEnv, args...)
}

// Run executes git in dir like the named operations do: hooks, the system and user config,
// filters and prompts are off, and output is trimmed. It is for the trusted operations of package
// trustedgit, which nightshift alone runs; the worker calls only the named operations here.
func (g *Git) Run(ctx context.Context, dir string, stdin []byte, extraEnv []string, args ...string) (string, error) {
	return g.run(ctx, dir, stdin, extraEnv, args...)
}

// RunUntrimmed is Run that keeps the output exactly as git wrote it, for output fed back to git.
func (g *Git) RunUntrimmed(ctx context.Context, dir string, stdin []byte, extraEnv []string, args ...string) (string, error) {
	return g.runRaw(ctx, nil, dir, stdin, extraEnv, args...)
}

// runGroups is run with supplementary groups for the user git runs as.
func (g *Git) runGroups(ctx context.Context, groups []uint32, dir string, stdin []byte, extraEnv []string, args ...string) (string, error) {
	out, err := g.runRaw(ctx, groups, dir, stdin, extraEnv, args...)
	return strings.TrimSpace(out), err
}

func (g *Git) runRaw(ctx context.Context, groups []uint32, dir string, stdin []byte, extraEnv []string, args ...string) (string, error) {
	// fsync: without it a power loss or a stopped VM leaves empty reference files behind.
	full := append([]string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "protocol.ext.allow=never", "-c", "core.fsync=all"}, args...)
	cmd := exec.CommandContext(ctx, g.bin(), full...) //nolint:gosec // g.bin is configuration; every argument is validated by the callers
	cmd.Dir = dir
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"), //nolint:forbidigo // git must be found; no other variable is passed through
		"HOME=/nonexistent", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_ATTR_NOSYSTEM=1",
	}, extraEnv...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if g.UID != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: g.UID, Gid: g.GID, Groups: groups}}
	}
	// A canceled git is asked to stop first: on SIGTERM it removes the lock files it holds
	// (`index.lock`, `*.lock` of references), and a SIGKILL would leave them in the mirror,
	// where every later git call would fail. Only a git that ignores the request is killed.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = stopGrace
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gitops: git %s: %w: %s", subcommand(args), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// subcommand is the git command in args, after any leading -c options.
func subcommand(args []string) string {
	for len(args) > 2 && args[0] == "-c" {
		args = args[2:]
	}
	return args[0]
}

// MirrorPath is the bare mirror of a repository ("owner/name") under Dir.
func (g *Git) MirrorPath(repo string) (string, error) {
	if !nameRe.MatchString(repo) || strings.Contains(repo, "..") {
		return "", fmt.Errorf("gitops: bad repository name %q", repo)
	}
	return filepath.Join(g.Dir, filepath.FromSlash(repo)+".git"), nil
}

const (
	stopGrace        = 10 * time.Second // how long a canceled git has to stop before it is killed
	checkoutAttempts = 3
)

// Checkout makes dest a working copy of mirror at sha, detached. It clones instead of
// adding a worktree because the worker mounts the mirror read-only.
func (g *Git) Checkout(ctx context.Context, mirror, dest, sha string) error {
	if !shaRe.MatchString(sha) {
		return fmt.Errorf("gitops: bad sha %q", sha)
	}
	// The mirror may belong to another user than the one git runs as; trust exactly this path.
	var groups []uint32
	if g.MirrorGID != 0 {
		groups = []uint32{g.MirrorGID}
	}
	// A local clone reads the mirror in a second git process, and some git versions do not hand
	// it the -c options, so that process gets the trust on its own command line.
	trust := "safe.directory=" + mirror
	// nightshift may swap the mirror while it is read; the reader cannot take its lock on a
	// read-only mount, so it tries again.
	var err error
	for attempt := range checkoutAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("gitops: %w", ctx.Err())
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		if _, err = g.runGroups(ctx, groups, filepath.Dir(dest), nil, nil, "-c", trust, "clone", "--no-hardlinks", "--no-checkout",
			"--upload-pack=git -c "+shellQuote(trust)+" upload-pack", "--", mirror, dest); err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	_, err = g.run(ctx, dest, nil, nil, "checkout", "--detach", sha)
	return err
}

// shellQuote makes s one word of the shell command git runs for --upload-pack.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Diff returns everything changed in the checkout since base as a binary-safe patch,
// including new files. The patch is data: nightshift applies it in its own copy.
func (g *Git) Diff(ctx context.Context, dir, base string) ([]byte, error) {
	if !shaRe.MatchString(base) {
		return nil, fmt.Errorf("gitops: bad sha %q", base)
	}
	if _, err := g.run(ctx, dir, nil, nil, "add", "-A"); err != nil {
		return nil, err
	}
	out, err := g.run(ctx, dir, nil, nil, "diff", "--cached", "--binary", "--no-ext-diff", base)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return []byte(out + "\n"), nil
}

// RemoveCheckout deletes a task checkout.
func (g *Git) RemoveCheckout(dest string) error {
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("gitops: %w", err)
	}
	return nil
}
