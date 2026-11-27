package trustedgit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ilyachakun/nightshift/internal/assurance/policy"
	"github.com/ilyachakun/nightshift/internal/gitops"
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
func (g *Git) Rebase(ctx context.Context, mirror, branch, base, expectedHead string, id Identity) (Rebased, error) {
	if !refRe.MatchString(branch) || !gitops.IsBranch(base) {
		return Rebased{}, fmt.Errorf("trustedgit: bad branch %q or base %q", branch, base)
	}
	old, err := g.Run(ctx, mirror, nil, nil, "rev-parse", "--verify", "refs/heads/"+branch)
	if err != nil {
		return Rebased{}, err
	}
	if old != expectedHead {
		return Rebased{}, errors.New("trustedgit: branch head changed before rebase")
	}
	onto, err := g.Run(ctx, mirror, nil, nil, "rev-parse", "--verify", "refs/heads/"+base)
	if err != nil {
		return Rebased{}, err
	}
	scratch, err := os.MkdirTemp("", "nightshift-rebase-")
	if err != nil {
		return Rebased{}, fmt.Errorf("trustedgit: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	work := filepath.Join(scratch, "w")
	if err := g.Checkout(ctx, mirror, work, old); err != nil {
		return Rebased{}, err
	}
	fork, err := g.Run(ctx, work, nil, nil, "merge-base", old, onto)
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
	if _, err := g.Run(ctx, work, nil, who, "rebase", "--no-verify", onto); err != nil {
		unmerged, uerr := g.Run(ctx, work, nil, nil, "diff", "--name-only", "--diff-filter=U")
		if uerr != nil || unmerged == "" {
			return Rebased{}, err
		}
		return Rebased{Conflict: true}, nil
	}
	sha, err := g.Run(ctx, work, nil, nil, "rev-parse", "HEAD")
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
	if _, err := g.Run(ctx, mirror, nil, nil, "fetch", "--force", work, "HEAD:refs/heads/"+branch); err != nil {
		return Rebased{}, err
	}
	return Rebased{SHA: sha, Identical: before == after}, nil
}

// PushRebase publishes the prepared commit only if the remote still has the approved head.
func (g *Git) PushRebase(ctx context.Context, mirror, remote, branch, expectedHead, candidate string, auth Auth) error {
	if !refRe.MatchString(branch) || !gitops.IsSHA(expectedHead) || !gitops.IsSHA(candidate) {
		return errors.New("trustedgit: invalid rebase publication")
	}
	ref := "refs/heads/" + branch
	_, err := g.Run(ctx, mirror, nil, auth.env(), "push", "--force-with-lease="+ref+":"+expectedHead, remote, candidate+":"+ref)
	return err
}

// RebaseOf reports whether head replays the commits of orig on the base branch: the same
// authors, author dates, committers and messages, in order, on top of a commit of the base,
// merges left out as a rebase drops them. This tells the factory's own rebase from an ordinary
// push; a writer who copies the metadata on purpose still meets the person who reviews the head.
func (g *Git) RebaseOf(ctx context.Context, mirror, head, orig, base string) (bool, error) {
	if !gitops.IsSHA(head) || !gitops.IsSHA(orig) || !gitops.IsBranch(base) {
		return false, fmt.Errorf("trustedgit: bad head %q, original %q or base %q", head, orig, base)
	}
	onto := "refs/heads/" + base
	commits := func(tip string) (string, error) {
		fork, err := g.Run(ctx, mirror, nil, nil, "merge-base", tip, onto)
		if err != nil {
			return "", err
		}
		return g.Run(ctx, mirror, nil, nil, "log", "--reverse", "--no-merges", "--format=%an%x00%ae%x00%at%x00%cn%x00%ce%x00%B%x00", fork+".."+tip)
	}
	was, err := commits(orig)
	if err != nil {
		return false, err
	}
	now, err := commits(head)
	if err != nil {
		return false, err
	}
	return was != "" && was == now, nil
}

// guardRebased runs the guard on what the rebased branch changes against its new base.
func (g *Git) guardRebased(ctx context.Context, work, mirror, onto, head string) error {
	if g.Guard == nil {
		return errors.New("trustedgit: no guard for rebases")
	}
	changed, err := g.Run(ctx, work, nil, nil, "diff", "--name-only", "-z", "--no-renames", onto, head)
	if err != nil {
		return err
	}
	if changed == "" {
		return nil // everything the branch changed is already in the base
	}
	return g.Guard(ctx, mirror, onto, strings.Split(strings.Trim(changed, "\x00"), "\x00"))
}

// patchID identifies the patch while preserving whitespace, including its final lines.
func (g *Git) patchID(ctx context.Context, dir, from, to string) (string, error) {
	diff, err := g.RunUntrimmed(ctx, dir, nil, nil, "diff", "--no-ext-diff", "--binary", from, to)
	if err != nil {
		return "", err
	}
	if diff == "" {
		return "", nil
	}
	out, err := g.Run(ctx, dir, []byte(diff), nil, "patch-id", "--verbatim")
	if err != nil {
		return "", err
	}
	id, _, _ := strings.Cut(out, " ")
	return id, nil
}

// Changes lists what a branch changes against the base: every path (a rename counts as
// both of its paths) and the number of changed lines. The classifier reads this, not an
// agent's account of its own patch.
func (g *Git) Changes(ctx context.Context, mirror, base, branch string) (paths []string, lines int, err error) {
	if !refRe.MatchString(branch) || !gitops.IsBranch(base) {
		return nil, 0, fmt.Errorf("trustedgit: bad branch %q or base %q", branch, base)
	}
	out, err := g.Run(ctx, mirror, nil, nil, "diff", "--numstat", "--no-renames", "-z", "refs/heads/"+base+"...refs/heads/"+branch)
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
