// Package gitops keeps repository mirrors, task checkouts and patch application. It runs
// the git binary with a scrubbed environment and no hooks, so repository content cannot
// run code in nightshift (design section 16).
package gitops

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	shaRe  = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)?$`)
	refRe  = regexp.MustCompile(`^ns/[A-Za-z0-9._-]+$`)
)

// Git runs git commands under Dir, the root of the `repos` volume.
type Git struct {
	Dir string
	Bin string // defaults to "git"
	// Guard sees every path a patch changes, after it is applied and before it is committed,
	// and may reject it (the constitution of design section 10). ApplyPatch needs it.
	Guard func(paths []string) error
	// UID and GID, when UID is set, run git as that user. The worker is root and must never
	// run git as root in a checkout the agent can write to: its config could run code.
	UID, GID uint32
	// MirrorGID, when UID is set, is a supplementary group of the clone alone: it lets the
	// worker read the mirrors nightshift keeps, which the agent itself must not browse.
	MirrorGID uint32
}

// Identity is the author of the commits nightshift writes.
type Identity struct{ Name, Email string }

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

// runGroups is run with supplementary groups for the user git runs as.
func (g *Git) runGroups(ctx context.Context, groups []uint32, dir string, stdin []byte, extraEnv []string, args ...string) (string, error) {
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
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gitops: git %s: %w: %s", subcommand(args), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
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

// EnsureMirror clones the repository as a bare mirror, or fetches into an existing one.
// remote may be a URL or a local path; auth is passed with Auth, never in the URL.
func (g *Git) EnsureMirror(ctx context.Context, repo, remote string, auth Auth) (string, error) {
	path, err := g.MirrorPath(repo)
	if err != nil {
		return "", err
	}
	env := auth.env()
	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		return path, g.clone(ctx, remote, path, env)
	}
	if err = g.convert(ctx, path); err == nil {
		_, err = g.run(ctx, path, nil, env, "fetch", "--prune", "--force", remote, "+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*")
	}
	if err == nil || !corrupt(err) {
		return path, err
	}
	// A mirror is a cache of the remote: keep the broken one for a look and clone anew, or
	// every task of the repository would fail on it forever.
	aside := path + ".broken-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if rerr := os.Rename(path, aside); rerr != nil {
		return path, errors.Join(err, fmt.Errorf("gitops: %w", rerr))
	}
	return path, g.clone(ctx, remote, path, env)
}

func (g *Git) clone(ctx context.Context, remote, path string, env []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("gitops: %w", err)
	}
	// Branches and tags only: a code host also publishes the commits of every pull request,
	// forks included, under refs of its own, and a checkout copies all objects of the mirror.
	// --no-local makes a clone of a local path transfer only what those refs reach.
	_, err := g.run(ctx, g.Dir, nil, env, "clone", "--bare", "--no-local", "--", remote, path)
	return err
}

// ConvertMirrors converts every mirror under Dir that still holds more than branches and
// tags. It needs no remote, so the service runs it before any job can read a mirror.
func (g *Git) ConvertMirrors(ctx context.Context) error {
	paths, err := filepath.Glob(filepath.Join(g.Dir, "*", "*.git"))
	if err != nil {
		return fmt.Errorf("gitops: %w", err)
	}
	var errs []error
	for _, path := range paths {
		errs = append(errs, g.convert(ctx, path))
	}
	return errors.Join(errs...)
}

// convert rebuilds a mirror that was cloned with every ref of its remote, from that mirror
// itself: deleting refs would leave their objects behind. The new clone is made beside the
// old one and swapped in only on success, and it keeps every branch, pushed or not.
func (g *Git) convert(ctx context.Context, path string) error {
	if !g.holdsMore(ctx, path) {
		return nil
	}
	fresh, old := path+".new", path+".old"
	for _, dir := range []string{fresh, old} { // left by an interrupted swap
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("gitops: %w", err)
		}
	}
	if err := g.clone(ctx, path, fresh, nil); err != nil {
		return err
	}
	if err := os.Rename(path, old); err != nil {
		return fmt.Errorf("gitops: %w", err)
	}
	if err := os.Rename(fresh, path); err != nil {
		return fmt.Errorf("gitops: %w", err)
	}
	if err := os.RemoveAll(old); err != nil {
		return fmt.Errorf("gitops: %w", err)
	}
	return nil
}

// holdsMore reports a mirror that was cloned as a git mirror, or has a ref that is neither a
// branch nor a tag. A mirror git cannot read reports false, and the fetch then finds out what
// is wrong with it.
func (g *Git) holdsMore(ctx context.Context, path string) bool {
	if v, err := g.run(ctx, path, nil, nil, "config", "--local", "--get", "remote.origin.mirror"); err == nil && v == "true" {
		return true
	}
	out, err := g.run(ctx, path, nil, nil, "for-each-ref", "--format=%(refname)")
	if err != nil {
		return false
	}
	for ref := range strings.Lines(out) {
		if !strings.HasPrefix(ref, "refs/heads/") && !strings.HasPrefix(ref, "refs/tags/") {
			return true
		}
	}
	return false
}

// corruptions are what git says about a repository damaged on disk, not about the remote.
var corruptions = []string{"reference broken", "bad object", "badrefcontent", "is corrupt", "object file", "not a git repository"}

func corrupt(err error) bool {
	msg := strings.ToLower(err.Error())
	return slices.ContainsFunc(corruptions, func(c string) bool { return strings.Contains(msg, c) })
}

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
	if _, err := g.runGroups(ctx, groups, filepath.Dir(dest), nil, nil, "-c", trust, "clone", "--no-hardlinks", "--no-checkout",
		"--upload-pack=git -c "+shellQuote(trust)+" upload-pack", "--", mirror, dest); err != nil {
		return err
	}
	_, err := g.run(ctx, dest, nil, nil, "checkout", "--detach", sha)
	return err
}

// shellQuote makes s one word of the shell command git runs for --upload-pack.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ApplyPatch applies a unified diff on base inside a scratch clone of the mirror, commits it
// as id and stores it as refs/heads/<branch> in the mirror. It returns the commit sha.
// The branch must be generated by nightshift (ns/<task>), never taken from a worker.
func (g *Git) ApplyPatch(ctx context.Context, mirror, branch, base string, patch []byte, id Identity, message string) (string, error) {
	if !refRe.MatchString(branch) {
		return "", fmt.Errorf("gitops: bad branch %q", branch)
	}
	if !shaRe.MatchString(base) {
		return "", fmt.Errorf("gitops: bad sha %q", base)
	}
	scratch, err := os.MkdirTemp("", "nightshift-apply-")
	if err != nil {
		return "", fmt.Errorf("gitops: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	work := filepath.Join(scratch, "w")
	if err := g.Checkout(ctx, mirror, work, base); err != nil {
		return "", err
	}
	if g.Guard == nil {
		return "", errors.New("gitops: no guard for patches")
	}
	if _, err := g.run(ctx, work, patch, nil, "apply", "--index", "--whitespace=nowarn", "-"); err != nil {
		return "", err
	}
	// Without renames a moved file lists both its old and new path; the applied result is
	// checked, so no way of writing a patch (renames, modes, quoting) hides a path.
	changed, err := g.run(ctx, work, nil, nil, "diff", "--cached", "--name-only", "-z", "--no-renames")
	if err != nil {
		return "", err
	}
	if err := g.Guard(strings.Split(strings.Trim(changed, "\x00"), "\x00")); err != nil {
		return "", err
	}
	who := []string{
		"GIT_AUTHOR_NAME=" + id.Name, "GIT_AUTHOR_EMAIL=" + id.Email,
		"GIT_COMMITTER_NAME=" + id.Name, "GIT_COMMITTER_EMAIL=" + id.Email,
	}
	if _, err := g.run(ctx, work, nil, who, "commit", "--no-verify", "--allow-empty-message", "-m", message); err != nil {
		return "", err
	}
	sha, err := g.run(ctx, work, nil, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if _, err := g.run(ctx, mirror, nil, nil, "fetch", "--force", work, "HEAD:refs/heads/"+branch); err != nil {
		return "", err
	}
	return sha, nil
}

// Push sends refs/heads/<branch> from the mirror to remote. Only nightshift writes its task
// branches, and a push to a URL has no remote-tracking ref for a lease to compare with, so the
// push is forced.
func (g *Git) Push(ctx context.Context, mirror, remote, branch string, auth Auth) error {
	if !refRe.MatchString(branch) {
		return fmt.Errorf("gitops: bad branch %q", branch)
	}
	_, err := g.run(ctx, mirror, nil, auth.env(), "push", "--force", remote, "refs/heads/"+branch+":refs/heads/"+branch)
	return err
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

// Auth carries an HTTPS token for GitHub. It goes to git through environment config, so
// it is neither in a URL nor in the process arguments.
type Auth struct{ Token string }

func (a Auth) env() []string {
	if a.Token == "" {
		return nil
	}
	return []string{
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraHeader",
		// Git over HTTPS takes a token only as the password of basic authentication.
		"GIT_CONFIG_VALUE_0=Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+a.Token)),
	}
}
