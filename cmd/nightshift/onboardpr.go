package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// onboardPR is everything needed to propose the onboarding files to a project as a pull
// request: nightshift writes the branch and the PR itself, never an agent (AGENTS.md rule 5).
type onboardPR struct {
	Repo, Base string
	Remote     string // clone and push URL of the repository
	Token      string
	Files      map[string][]byte // path in the repository -> content; new files only
	Body       string
	Forge      integration.PullRequests
}

const onboardBranch = "ns/onboard"

// openOnboardingPR pushes the files as one commit on ns/onboard and opens a draft pull
// request. The owner reads and merges it; nothing is merged here.
func openOnboardingPR(ctx context.Context, p onboardPR) (integration.PRRef, error) {
	dir, err := os.MkdirTemp("", "nightshift-onboard-")
	if err != nil {
		return integration.PRRef{}, fmt.Errorf("onboard: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	// The constitution protects .nightshift/ from Builder patches; this change is the owner's
	// own onboarding and goes through a pull request they review, so the guard lets it pass.
	git := &gitops.Git{Dir: dir, Guard: func([]string) error { return nil }}
	auth := gitops.Auth{Token: p.Token}
	mirror, err := git.EnsureMirror(ctx, p.Repo, p.Remote, auth)
	if err != nil {
		return integration.PRRef{}, fmt.Errorf("onboard: %w", err)
	}
	base, err := git.Rev(ctx, mirror, p.Base)
	if err != nil {
		return integration.PRRef{}, fmt.Errorf("onboard: %w", err)
	}
	id := gitops.Identity{Name: "nightshift", Email: "nightshift@users.noreply.github.com"}
	if _, err = git.ApplyPatch(ctx, mirror, onboardBranch, base, newFilesPatch(p.Files), id, "chore: add Nightshift project instructions"); err != nil {
		return integration.PRRef{}, fmt.Errorf("onboard: %w", err)
	}
	if err = git.Push(ctx, mirror, p.Remote, onboardBranch, auth); err != nil {
		return integration.PRRef{}, fmt.Errorf("onboard: %w", err)
	}
	return p.Forge.OpenPR(ctx, integration.OpenPR{
		Repo: p.Repo, Head: onboardBranch, Base: p.Base, Draft: true,
		Title: "chore: add Nightshift project instructions", Body: p.Body,
	})
}

// newFilesPatch is a unified diff that creates each file.
func newFilesPatch(files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		text := string(files[n])
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
		fmt.Fprintf(&b, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", n, n, n, len(lines))
		for _, l := range lines {
			b.WriteString("+" + l + "\n")
		}
	}
	return []byte(b.String())
}
