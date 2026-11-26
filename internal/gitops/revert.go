package gitops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Revert commits the revert of sha on base inside a scratch clone of the mirror and stores it as
// refs/heads/<branch> in the mirror (design section 7, "Rollback"). A merge commit is reverted
// against its first parent, the base branch before the merge. It returns the new commit.
func (g *Git) Revert(ctx context.Context, mirror, branch, base, sha string, id Identity) (string, error) {
	if !refRe.MatchString(branch) {
		return "", fmt.Errorf("gitops: bad branch %q", branch)
	}
	if !shaRe.MatchString(base) || !shaRe.MatchString(sha) {
		return "", fmt.Errorf("gitops: bad sha %q or %q", base, sha)
	}
	scratch, err := os.MkdirTemp("", "nightshift-revert-")
	if err != nil {
		return "", fmt.Errorf("gitops: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	work := filepath.Join(scratch, "w")
	if err := g.Checkout(ctx, mirror, work, base); err != nil {
		return "", err
	}
	parents, err := g.run(ctx, work, nil, nil, "rev-list", "--parents", "-n", "1", sha)
	if err != nil {
		return "", err
	}
	args := []string{"revert", "--no-edit"}
	switch n := len(strings.Fields(parents)) - 1; {
	case n == 0:
		return "", errors.New("gitops: a root commit cannot be reverted")
	case n > 1:
		args = append(args, "-m", "1")
	}
	who := []string{
		"GIT_AUTHOR_NAME=" + id.Name, "GIT_AUTHOR_EMAIL=" + id.Email,
		"GIT_COMMITTER_NAME=" + id.Name, "GIT_COMMITTER_EMAIL=" + id.Email,
	}
	if _, err := g.run(ctx, work, nil, who, append(args, sha)...); err != nil {
		return "", fmt.Errorf("gitops: revert %s: %w", sha, err)
	}
	head, err := g.run(ctx, work, nil, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if _, err := g.run(ctx, mirror, nil, nil, "fetch", "--force", work, "HEAD:refs/heads/"+branch); err != nil {
		return "", err
	}
	return head, nil
}
