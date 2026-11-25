package dispatch

import (
	"context"
	"fmt"

	"github.com/ilyachakun/nightshift/internal/gitops"
)

// mirror clones or updates nightshift's mirror of a repository.
func (c *Conductor) mirror(ctx context.Context, repo string) (string, error) {
	auth, err := c.auth(ctx, repo)
	if err != nil {
		return "", err
	}
	path, err := c.Git.EnsureMirror(ctx, repo, c.Remote(repo), auth)
	if err != nil {
		return "", fmt.Errorf("conductor: mirror of %s: %w", repo, err)
	}
	return path, nil
}

// pushBranch sends a task branch from the mirror to the repository.
func (c *Conductor) pushBranch(ctx context.Context, mirror, repo, branch string) error {
	auth, err := c.auth(ctx, repo)
	if err != nil {
		return err
	}
	if err := c.Git.Push(ctx, mirror, c.Remote(repo), branch, auth); err != nil {
		return fmt.Errorf("conductor: push: %w", err)
	}
	return nil
}

func (c *Conductor) auth(ctx context.Context, repo string) (gitops.Auth, error) {
	if c.Auth == nil {
		return gitops.Auth{}, nil
	}
	auth, err := c.Auth(ctx, repo)
	if err != nil {
		return gitops.Auth{}, fmt.Errorf("conductor: credential for %s: %w", repo, err)
	}
	return auth, nil
}
