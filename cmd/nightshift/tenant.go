package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/ilyachakun/nightshift/internal/adapters/github"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/customization/onboard"
)

var tenantName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// tenantCmd handles `nightshift tenant`: discovery, the pull request that proposes the
// project file, and the doctor.
func tenantCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "doctor" {
		return tenantDoctor(args[1:], stdout, stderr)
	}
	if len(args) == 0 || args[0] != "init" {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant: usage: tenant init <name> --discover --repo-dir <checkout> [--out <dir>] | tenant doctor <name> --repo-dir <checkout>")
		return exitUsage
	}
	fs := flag.NewFlagSet("tenant init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	discover := fs.Bool("discover", false, "read the repository and write drafts into the onboarding directory")
	repoDir := fs.String("repo-dir", "", "checkout of the tenant's repository (read only)")
	out := fs.String("out", "", "where the drafts go; default tenants/<name>/onboarding")
	openPR := fs.String("open-pr", "", "owner/repo: propose .nightshift/project.yaml to this repository as a draft pull request")
	base := fs.String("base", "main", "base branch of the pull request")
	stacksDir := fs.String("stacks", "stacks", "stack packs that tell which stacks the repository uses; a missing directory means a built-in list")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with compose secrets (github_token)")
	rest := args[1:]
	if len(rest) == 0 {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant init: the tenant name is missing")
		return exitUsage
	}
	name := rest[0]
	if err := fs.Parse(rest[1:]); err != nil {
		return exitUsage
	}
	if !tenantName.MatchString(name) {
		_, _ = fmt.Fprintf(stderr, "nightshift tenant init: %q is not a tenant name (lowercase letters, digits, dashes)\n", name)
		return exitUsage
	}
	if !*discover {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant init: only --discover is implemented yet")
		return exitNotImplemented
	}
	if *repoDir == "" {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant init: --repo-dir is required")
		return exitUsage
	}
	dest := *out
	if dest == "" {
		dest = filepath.Join("tenants", name, "onboarding")
	}
	packs, err := loadStacks(*stacksDir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant init:", err)
		return exitFailure
	}
	rep := onboard.Discover(os.DirFS(*repoDir), name, packs)
	project, err := rep.ProjectYAML()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant init:", err)
		return exitFailure
	}
	if err := os.MkdirAll(dest, 0o750); err != nil { //nolint:gosec // the owner chooses the output directory on the command line
		_, _ = fmt.Fprintln(stderr, "nightshift tenant init:", err)
		return exitFailure
	}
	for file, body := range map[string][]byte{"project.yaml": project, "gaps.md": []byte(rep.GapsMD())} {
		if err := os.WriteFile(filepath.Join(dest, file), body, 0o600); err != nil { //nolint:gosec // the owner chooses the output directory on the command line
			_, _ = fmt.Fprintln(stderr, "nightshift tenant init:", err)
			return exitFailure
		}
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s and %s; next: run the onboard-tenant skill on them\n",
		filepath.Join(dest, "project.yaml"), filepath.Join(dest, "gaps.md"))
	if *openPR == "" {
		return exitOK
	}
	token, err := config.Secrets{Dir: *secrets}.Read(secretGitHubToken)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant init:", err)
		return exitFailure
	}
	ref, err := openOnboardingPR(context.Background(), onboardPR{
		Repo: *openPR, Base: *base, Remote: "https://github.com/" + *openPR + ".git", Token: token,
		Files: map[string][]byte{".nightshift/project.yaml": project}, Body: rep.GapsMD(),
		Forge: &github.Client{Token: token},
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant init:", err)
		return exitFailure
	}
	_, _ = fmt.Fprintf(stdout, "opened draft pull request #%d in %s\n", ref.Number, *openPR)
	return exitOK
}

// tenantDoctor runs the connection check of design section 17, step 7.
func tenantDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tenant doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("config", "/etc/nightshift", "directory with instance.yaml")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with compose secrets")
	repoDir := fs.String("repo-dir", "", "checkout of the tenant's repository (read only)")
	stacksDir := fs.String("stacks", "stacks", "stack packs; a missing directory means the built-in toolchain list")
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant doctor: the tenant name is missing")
		return exitUsage
	}
	name := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	if *repoDir == "" {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant doctor: --repo-dir is required")
		return exitUsage
	}
	v := config.NewValidator()
	inst, err := config.LoadInstance(v, *dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant doctor:", err)
		return exitFailure
	}
	t, ok := inst.Tenants[name]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "nightshift tenant doctor: instance %s has no tenant %q\n", inst.Name, name)
		return exitFailure
	}
	packs, err := loadStacks(*stacksDir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant doctor:", err)
		return exitFailure
	}
	sec := config.Secrets{Dir: *secrets}
	rs := onboard.Doctor{
		Packs: packs, Tenant: t, Repo: os.DirFS(*repoDir), Validator: v,
		Secret: func(n string) error { _, err := sec.Read(n); return err },
		Look:   func(n string) error { _, err := exec.LookPath(n); return err },
	}.Run()
	_, _ = fmt.Fprint(stdout, onboard.Format(rs))
	if onboard.Failed(rs) {
		return exitFailure
	}
	return exitOK
}
