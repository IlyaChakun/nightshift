package gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var baseRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

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
	after, err := g.patchID(ctx, work, onto, sha)
	if err != nil {
		return Rebased{}, err
	}
	if _, err := g.run(ctx, mirror, nil, nil, "fetch", "--force", work, "HEAD:refs/heads/"+branch); err != nil {
		return Rebased{}, err
	}
	return Rebased{SHA: sha, Identical: before == after}, nil
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
