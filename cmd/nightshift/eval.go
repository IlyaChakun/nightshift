package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/ilyachakun/nightshift/internal/evals"
)

// evalCmd is `nightshift eval`. Only `select` exists so far: it prints the evals a change
// triggers. Running them needs the polygon (design section 20) and is not built yet.
func evalCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "select" {
		_, _ = fmt.Fprintln(stderr, "usage: nightshift eval select [--rules evals/selection.yaml] [--base COMMIT | path...]\n(`eval run` waits for the polygon, design phase 4)")
		if len(args) > 0 && args[0] == "run" {
			return exitNotImplemented
		}
		return exitUsage
	}
	fs := flag.NewFlagSet("eval select", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rulesPath := fs.String("rules", "evals/selection.yaml", "rules that map changed paths to evals")
	base := fs.String("base", "", "take the changed paths from git diff against this commit")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	doc, err := os.ReadFile(*rulesPath) //nolint:gosec // G304: the rules file is named by the operator
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval select: %v\n", err)
		return exitFailure
	}
	rules, err := evals.Parse(doc)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval select: %v\n", err)
		return exitFailure
	}
	changed := fs.Args()
	if *base != "" {
		if strings.HasPrefix(*base, "-") {
			_, _ = fmt.Fprintln(stderr, "nightshift eval select: --base must be a commit, not an option")
			return exitUsage
		}
		out, err := exec.CommandContext(context.Background(), "git", "diff", "--name-only", "--no-renames", *base+"...HEAD", "--").Output() //nolint:gosec // G204: a fixed program; the base is an argument of git diff, not a shell word
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "nightshift eval select: git diff: %v\n", err)
			return exitFailure
		}
		changed = append(changed, strings.Fields(string(out))...)
	}
	runs := evals.Select(rules, changed)
	if len(runs) == 0 {
		_, _ = fmt.Fprintln(stdout, "no evals: this change is covered by tests")
		return exitOK
	}
	for _, r := range runs {
		role := r.Role
		if role == "" {
			role = "-"
		}
		_, _ = fmt.Fprintf(stdout, "%s %s %s\n", r.Set, r.Level, role)
	}
	return exitOK
}
