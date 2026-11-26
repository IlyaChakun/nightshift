package gitops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ilyachakun/nightshift/internal/policy"
)

var (
	baseRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	// pathRe is a repository path; dot directories such as .nightshift are fine, `..` is not.
	pathRe = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._/-]*$`)
)

// Rebased is the outcome of rebasing a task branch on the base branch.
type Rebased struct {
	SHA string
	// Conflict means the rebase stopped on a conflict; the branch is unchanged.
	Conflict bool
	// Identical means the change itself is the same as before the rebase (equal patch-ids),
	// so approvals and verdicts given to the old commit carry over (design section 7).
	Identical bool
}

// Rebase puts refs/heads/<branch> of the mirror on top of refs/heads/<base> in a scratch
// clone and, when it applies cleanly, stores the result under the same branch name.
func (g *Git) Rebase(ctx context.Context, mirror, branch, base string, id Identity) (Rebased, error) {
	if !refRe.MatchString(branch) || !baseRe.MatchString(base) {
		return Rebased{}, fmt.Errorf("gitops: bad branch %q or base %q", branch, base)
	}
	old, err := g.run(ctx, mirror, nil, nil, "rev-parse", "--verify", "refs/heads/"+branch)
	if err != nil {
		return Rebased{}, err
	}
	onto, err := g.run(ctx, mirror, nil, nil, "rev-parse", "--verify", "refs/heads/"+base)
	if err != nil {
		return Rebased{}, err
	}
	scratch, err := os.MkdirTemp("", "nightshift-rebase-")
	if err != nil {
		return Rebased{}, fmt.Errorf("gitops: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	work := filepath.Join(scratch, "w")
	if err := g.Checkout(ctx, mirror, work, old); err != nil {
		return Rebased{}, err
	}
	fork, err := g.run(ctx, work, nil, nil, "merge-base", old, onto)
	if err != nil {
		return Rebased{}, err
	}
	before, err := g.patchID(ctx, work, fork, old)
	if err != nil {
		return Rebased{}, err
	}
	who := []string{
		"GIT_AUTHOR_NAME=" + id.Name, "GIT_AUTHOR_EMAIL=" + id.Email,
		"GIT_COMMITTER_NAME=" + id.Name, "GIT_COMMITTER_EMAIL=" + id.Email,
	}
	if _, err := g.run(ctx, work, nil, who, "rebase", "--no-verify", onto); err != nil {
		unmerged, uerr := g.run(ctx, work, nil, nil, "diff", "--name-only", "--diff-filter=U")
		if uerr != nil || unmerged == "" {
			return Rebased{}, err
		}
		return Rebased{Conflict: true}, nil
	}
	sha, err := g.run(ctx, work, nil, nil, "rev-parse", "HEAD")
	if err != nil {
		return Rebased{}, err
	}
	// The base moved: a path the project protects now may have been unprotected when the patch
	// was applied. The branch stays where it was and goes back to the Builder, whose next
	// patch meets the guard on the new base.
	if err := g.guardRebased(ctx, work, mirror, onto, sha); errors.Is(err, policy.ErrProtected) {
		return Rebased{Conflict: true}, nil
	} else if err != nil {
		return Rebased{}, err
	}
	after, err := g.patchID(ctx, work, onto, sha)
	if err != nil {
		return Rebased{}, err
	}
	if _, err := g.run(ctx, mirror, nil, nil, "fetch", "--force", work, "HEAD:refs/heads/"+branch); err != nil {
		return Rebased{}, err
	}
	return Rebased{SHA: sha, Identical: before == after}, nil
}

// guardRebased runs the guard on what the rebased branch changes against its new base.
func (g *Git) guardRebased(ctx context.Context, work, mirror, onto, head string) error {
	if g.Guard == nil {
		return errors.New("gitops: no guard for rebases")
	}
	changed, err := g.run(ctx, work, nil, nil, "diff", "--name-only", "-z", "--no-renames", onto, head)
	if err != nil {
		return err
	}
	if changed == "" {
		return nil // everything the branch changed is already in the base
	}
	return g.Guard(ctx, mirror, onto, strings.Split(strings.Trim(changed, "\x00"), "\x00"))
}

// patchID is the stable patch-id of the change between two commits, its content identity.
func (g *Git) patchID(ctx context.Context, dir, from, to string) (string, error) {
	diff, err := g.run(ctx, dir, nil, nil, "diff", "--no-ext-diff", "--binary", from, to)
	if err != nil {
		return "", err
	}
	if diff == "" {
		return "", nil
	}
	out, err := g.run(ctx, dir, []byte(diff+"\n"), nil, "patch-id", "--stable")
	if err != nil {
		return "", err
	}
	id, _, _ := strings.Cut(out, " ")
	return id, nil
}

// Rev returns the commit a branch of the mirror points at.
func (g *Git) Rev(ctx context.Context, mirror, branch string) (string, error) {
	if !baseRe.MatchString(branch) {
		return "", fmt.Errorf("gitops: bad branch %q", branch)
	}
	return g.run(ctx, mirror, nil, nil, "rev-parse", "--verify", "refs/heads/"+branch)
}

// Changes lists what a branch changes against the base: every path (a rename counts as
// both of its paths) and the number of changed lines. The classifier reads this, not an
// agent's account of its own patch.
func (g *Git) Changes(ctx context.Context, mirror, base, branch string) (paths []string, lines int, err error) {
	if !refRe.MatchString(branch) || !baseRe.MatchString(base) {
		return nil, 0, fmt.Errorf("gitops: bad branch %q or base %q", branch, base)
	}
	out, err := g.run(ctx, mirror, nil, nil, "diff", "--numstat", "--no-renames", "-z", "refs/heads/"+base+"...refs/heads/"+branch)
	if err != nil {
		return nil, 0, err
	}
	for rec := range strings.SplitSeq(out, "\x00") {
		f := strings.SplitN(rec, "\t", 3)
		if len(f) != 3 {
			continue
		}
		paths = append(paths, f[2])
		var add, del int
		// A binary file shows "-"; it counts as zero lines and still as a path.
		_, _ = fmt.Sscanf(f[0], "%d", &add)
		_, _ = fmt.Sscanf(f[1], "%d", &del)
		lines += add + del
	}
	return paths, lines, nil
}

// Show returns a file of the mirror at a commit, and false when the commit has no such file.
func (g *Git) Show(ctx context.Context, mirror, sha, name string) (string, bool, error) {
	if !shaRe.MatchString(sha) || !pathRe.MatchString(name) || strings.Contains(name, "..") {
		return "", false, fmt.Errorf("gitops: bad commit %q or path %q", sha, name)
	}
	entry, err := g.run(ctx, mirror, nil, nil, "ls-tree", "--name-only", sha, "--", name)
	if err != nil || entry == "" {
		return "", false, err
	}
	out, err := g.run(ctx, mirror, nil, nil, "show", sha+":"+name)
	return out, err == nil, err
}

// ObjectKind is the type of a path at a commit ("blob" for a file, "tree" for a directory);
// an empty string means the path does not exist.
func (g *Git) ObjectKind(ctx context.Context, mirror, sha, name string) (string, error) {
	if !shaRe.MatchString(sha) || !pathRe.MatchString(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("gitops: bad commit %q or path %q", sha, name)
	}
	out, err := g.run(ctx, mirror, nil, nil, "cat-file", "-t", sha+":"+name)
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
	out, err := g.run(ctx, mirror, nil, nil, "ls-tree", sha, "--", name)
	if err != nil {
		return false, nil //nolint:nilerr // a missing path is the answer, not a failure
	}
	mode, _, _ := strings.Cut(strings.TrimSpace(out), " ")
	return mode == "100644" || mode == "100755", nil
}
