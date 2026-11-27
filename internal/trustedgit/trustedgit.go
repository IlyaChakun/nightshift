// Package trustedgit holds the Git operations only nightshift runs: it fetches the mirrors with
// the code host's token, applies a worker's patch behind the guard, pushes task branches, rebases
// and reverts. The worker never imports it; its half of Git is package gitops.
package trustedgit

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ilyachakun/nightshift/internal/gitops"
)

var refRe = regexp.MustCompile(`^ns/[A-Za-z0-9._-]+$`)

const lockRetry = 50 * time.Millisecond

// Git runs the trusted operations on the mirrors of the embedded worker half.
type Git struct {
	*gitops.Git
	// Guard sees every path a patch changes, after it is applied and before it is committed,
	// and may reject it (the constitution of design section 10). It also gets the mirror and the
	// base commit, so a project's own rules can be read at the base, never from the patch.
	// ApplyPatch needs it.
	Guard func(ctx context.Context, mirror, base string, paths []string) error
}

// Identity is the author of the commits nightshift writes.
type Identity struct{ Name, Email string }

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

// EnsureMirror clones the repository as a bare mirror, or fetches into an existing one.
// remote may be a URL or a local path; auth is passed with Auth, never in the URL.
func (g *Git) EnsureMirror(ctx context.Context, repo, remote string, auth Auth) (string, error) {
	path, err := g.MirrorPath(repo)
	if err != nil {
		return "", err
	}
	unlock, err := lockMirror(ctx, path)
	if err != nil {
		return path, err
	}
	defer unlock()
	env := auth.env()
	if err := restore(path); err != nil {
		return path, err
	}
	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		return path, g.clone(ctx, remote, path, env)
	}
	if err = g.convert(ctx, path); err == nil {
		_, err = g.Run(ctx, path, nil, env, "fetch", "--prune", "--force", remote, "+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*")
	}
	if err == nil || !corrupt(err) {
		return path, err
	}
	// A mirror is a cache of the remote: keep the broken one for a look and clone anew, or
	// every task of the repository would fail on it forever.
	aside := path + ".broken-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if rerr := os.Rename(path, aside); rerr != nil {
		return path, errors.Join(err, fmt.Errorf("trustedgit: %w", rerr))
	}
	return path, g.clone(ctx, remote, path, env)
}

func (g *Git) clone(ctx context.Context, remote, path string, env []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("trustedgit: %w", err)
	}
	// Branches and tags only: a code host also publishes the commits of every pull request,
	// forks included, under refs of its own, and a checkout copies all objects of the mirror.
	// --no-local makes a clone of a local path transfer only what those refs reach.
	_, err := g.Run(ctx, g.Dir, nil, env, "clone", "--bare", "--no-local", "--", remote, path)
	return err
}

// ConvertMirrors converts every mirror under Dir that still holds more than branches and
// tags. It needs no remote, so the service runs it before any job can read a mirror. A mirror
// it cannot convert is moved out of the way: the next use clones it anew.
func (g *Git) ConvertMirrors(ctx context.Context) error {
	paths, err := filepath.Glob(filepath.Join(g.Dir, "*", "*.git"))
	if err != nil {
		return fmt.Errorf("trustedgit: %w", err)
	}
	// A swap may have left a mirror only under its old name.
	left, err := filepath.Glob(filepath.Join(g.Dir, "*", "*.git.old"))
	if err != nil {
		return fmt.Errorf("trustedgit: %w", err)
	}
	for _, old := range left {
		if path := strings.TrimSuffix(old, ".old"); !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	var errs []error
	for _, path := range paths {
		errs = append(errs, g.convertLocked(ctx, path))
	}
	return errors.Join(errs...)
}

func (g *Git) convertLocked(ctx context.Context, path string) error {
	unlock, err := lockMirror(ctx, path)
	if err != nil {
		return err
	}
	defer unlock()
	if err := restore(path); err != nil {
		return err
	}
	err = g.convert(ctx, path)
	// A canceled start serves no jobs, so it leaves the mirror for the next one.
	if err == nil || ctx.Err() != nil || !g.holdsMore(ctx, path) {
		return err
	}
	// The worker must not read it. It is kept: it may hold a branch that was not pushed.
	aside := path + ".unconverted-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if rerr := os.Rename(path, aside); rerr != nil {
		return errors.Join(err, fmt.Errorf("trustedgit: %w", rerr))
	}
	return fmt.Errorf("trustedgit: mirror moved to %s: %w", aside, err)
}

// lockMirror holds the mirror for one writer, in this process or another: every open of the
// lock file is a holder of its own.
func lockMirror(ctx context.Context, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("trustedgit: %w", err)
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // the path is a validated mirror path
	if err != nil {
		return nil, fmt.Errorf("trustedgit: %w", err)
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = f.Close() }, nil // closing releases the lock
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("trustedgit: lock %s: %w", path, err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("trustedgit: lock %s: %w", path, ctx.Err())
		case <-time.After(lockRetry):
		}
	}
}

// restore puts back a mirror that a swap moved away and never replaced, for example because
// the service stopped between the two renames: it may hold a branch that was not pushed.
func restore(path string) error {
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return nil // another stat error shows up at the next git command
	}
	if err := os.Rename(path+".old", path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("trustedgit: %w", err)
	}
	return nil
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
			return fmt.Errorf("trustedgit: %w", err)
		}
	}
	if err := g.clone(ctx, path, fresh, nil); err != nil {
		return errors.Join(err, os.RemoveAll(fresh))
	}
	if err := os.Rename(path, old); err != nil {
		return fmt.Errorf("trustedgit: %w", err)
	}
	if err := os.Rename(fresh, path); err != nil {
		return errors.Join(fmt.Errorf("trustedgit: %w", err), os.Rename(old, path), os.RemoveAll(fresh))
	}
	if err := os.RemoveAll(old); err != nil {
		return fmt.Errorf("trustedgit: %w", err)
	}
	return nil
}

// holdsMore reports a mirror that was cloned as a git mirror, or has a ref that is neither a
// branch nor a tag. A mirror git cannot read reports false, and the fetch then finds out what
// is wrong with it.
func (g *Git) holdsMore(ctx context.Context, path string) bool {
	if v, err := g.Run(ctx, path, nil, nil, "config", "--local", "--get", "remote.origin.mirror"); err == nil && v == "true" {
		return true
	}
	out, err := g.Run(ctx, path, nil, nil, "for-each-ref", "--format=%(refname)")
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

// ApplyPatch applies a unified diff on base inside a scratch clone of the mirror, commits it
// as id and stores it as refs/heads/<branch> in the mirror. It returns the commit sha.
// The branch must be generated by nightshift (ns/<task>), never taken from a worker.
func (g *Git) ApplyPatch(ctx context.Context, mirror, branch, base string, patch []byte, id Identity, message string) (string, error) {
	if !refRe.MatchString(branch) {
		return "", fmt.Errorf("trustedgit: bad branch %q", branch)
	}
	if !gitops.IsSHA(base) {
		return "", fmt.Errorf("trustedgit: bad sha %q", base)
	}
	scratch, err := os.MkdirTemp("", "nightshift-apply-")
	if err != nil {
		return "", fmt.Errorf("trustedgit: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	work := filepath.Join(scratch, "w")
	if err := g.Checkout(ctx, mirror, work, base); err != nil {
		return "", err
	}
	if g.Guard == nil {
		return "", errors.New("trustedgit: no guard for patches")
	}
	if _, err := g.Run(ctx, work, patch, nil, "apply", "--index", "--whitespace=nowarn", "-"); err != nil {
		return "", err
	}
	// Without renames a moved file lists both its old and new path; the applied result is
	// checked, so no way of writing a patch (renames, modes, quoting) hides a path.
	changed, err := g.Run(ctx, work, nil, nil, "diff", "--cached", "--name-only", "-z", "--no-renames")
	if err != nil {
		return "", err
	}
	if err := g.Guard(ctx, mirror, base, strings.Split(strings.Trim(changed, "\x00"), "\x00")); err != nil {
		return "", err
	}
	who := []string{
		"GIT_AUTHOR_NAME=" + id.Name, "GIT_AUTHOR_EMAIL=" + id.Email,
		"GIT_COMMITTER_NAME=" + id.Name, "GIT_COMMITTER_EMAIL=" + id.Email,
	}
	if _, err := g.Run(ctx, work, nil, who, "commit", "--no-verify", "--allow-empty-message", "-m", message); err != nil {
		return "", err
	}
	sha, err := g.Run(ctx, work, nil, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if _, err := g.Run(ctx, mirror, nil, nil, "fetch", "--force", work, "HEAD:refs/heads/"+branch); err != nil {
		return "", err
	}
	return sha, nil
}

// Push sends refs/heads/<branch> from the mirror to remote. Only nightshift writes its task
// branches, and a push to a URL has no remote-tracking ref for a lease to compare with, so the
// push is forced.
func (g *Git) Push(ctx context.Context, mirror, remote, branch string, auth Auth) error {
	if !refRe.MatchString(branch) {
		return fmt.Errorf("trustedgit: bad branch %q", branch)
	}
	_, err := g.Run(ctx, mirror, nil, auth.env(), "push", "--force", remote, "refs/heads/"+branch+":refs/heads/"+branch)
	return err
}
