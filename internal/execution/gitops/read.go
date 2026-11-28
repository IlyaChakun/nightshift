package gitops

import (
	"context"
	"fmt"
	"strings"
)

// read runs a read-only git command in a mirror. The mirror belongs to nightshift, and a caller
// running as another user (the deployer runs as root) would be refused by git's ownership check,
// so exactly this path is trusted, as Checkout does.
func (g *Git) read(ctx context.Context, mirror string, args ...string) (string, error) {
	return g.run(ctx, mirror, nil, nil, append([]string{"-c", "safe.directory=" + mirror}, args...)...)
}

// Rev returns the commit a branch of the mirror points at.
func (g *Git) Rev(ctx context.Context, mirror, branch string) (string, error) {
	if !baseRe.MatchString(branch) {
		return "", fmt.Errorf("gitops: bad branch %q", branch)
	}
	return g.read(ctx, mirror, "rev-parse", "--verify", "refs/heads/"+branch)
}

// Show returns a file of the mirror at a commit, and false when the commit has no such file.
func (g *Git) Show(ctx context.Context, mirror, sha, name string) (string, bool, error) {
	if !shaRe.MatchString(sha) || !pathRe.MatchString(name) || strings.Contains(name, "..") {
		return "", false, fmt.Errorf("gitops: bad commit %q or path %q", sha, name)
	}
	entry, err := g.read(ctx, mirror, "ls-tree", "--name-only", sha, "--", name)
	if err != nil || entry == "" {
		return "", false, err
	}
	out, err := g.read(ctx, mirror, "show", sha+":"+name)
	return out, err == nil, err
}

// ObjectKind is the type of a path at a commit ("blob" for a file, "tree" for a directory);
// an empty string means the path does not exist.
func (g *Git) ObjectKind(ctx context.Context, mirror, sha, name string) (string, error) {
	if !shaRe.MatchString(sha) || !pathRe.MatchString(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("gitops: bad commit %q or path %q", sha, name)
	}
	out, err := g.read(ctx, mirror, "cat-file", "-t", sha+":"+name)
	if err != nil {
		return "", nil //nolint:nilerr // a missing path is the answer, not a failure
	}
	return strings.TrimSpace(out), nil
}

// IsRegularFile reports whether the path is an ordinary file at the commit: not a directory, a
// symlink or a submodule.
func (g *Git) IsRegularFile(ctx context.Context, mirror, sha, name string) (bool, error) {
	if !shaRe.MatchString(sha) || !pathRe.MatchString(name) || strings.Contains(name, "..") {
		return false, fmt.Errorf("gitops: bad commit %q or path %q", sha, name)
	}
	out, err := g.read(ctx, mirror, "ls-tree", sha, "--", name)
	if err != nil {
		return false, nil //nolint:nilerr // a missing path is the answer, not a failure
	}
	mode, _, _ := strings.Cut(strings.TrimSpace(out), " ")
	return mode == "100644" || mode == "100755", nil
}
