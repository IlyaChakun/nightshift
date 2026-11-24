package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/ilyachakun/nightshift/internal/onboard"
)

var tenantName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// tenantCmd handles `nightshift tenant`. Only discovery exists so far; init proper and
// doctor come with the next steps of phase 3.
func tenantCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "init" {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant: usage: tenant init <name> --discover --repo-dir <checkout> [--out <dir>]")
		return exitUsage
	}
	fs := flag.NewFlagSet("tenant init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	discover := fs.Bool("discover", false, "read the repository and write drafts into the onboarding directory")
	repoDir := fs.String("repo-dir", "", "checkout of the tenant's repository (read only)")
	out := fs.String("out", "", "where the drafts go; default tenants/<name>/onboarding")
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
	rep := onboard.Discover(os.DirFS(*repoDir), name)
	project, err := rep.ProjectYAML()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant init:", err)
		return exitFailure
	}
	if err := os.MkdirAll(dest, 0o750); err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift tenant init:", err)
		return exitFailure
	}
	for file, body := range map[string][]byte{"project.yaml": project, "gaps.md": []byte(rep.GapsMD())} {
		if err := os.WriteFile(filepath.Join(dest, file), body, 0o600); err != nil {
			_, _ = fmt.Fprintln(stderr, "nightshift tenant init:", err)
			return exitFailure
		}
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s and %s; next: run the onboard-tenant skill on them\n",
		filepath.Join(dest, "project.yaml"), filepath.Join(dest, "gaps.md"))
	return exitOK
}
