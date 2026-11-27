package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/ilyachakun/nightshift/internal/testing/evals"
)

// evalCmd is `nightshift eval`: `select` prints the evals a change triggers, `run` solves the
// polygon tasks locally (design section 10, "The polygon").
func evalCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "run" {
		return evalRun(args[1:], stdout, stderr)
	}
	if len(args) == 0 || args[0] != "select" {
		_, _ = fmt.Fprintln(stderr, "usage: nightshift eval select [--rules evals/selection.yaml] [--base COMMIT | path...]\n       nightshift eval run [--role lead [--cases evals/lead/cases.yaml] [--roles roles] [--skills skills]] [--tasks evals/tasks] [--set fast] [--harness fake --scenario FILE | --harness claude --auth api_key] [--candidate NAME] [--out eval-runs]")
		return exitUsage
	}
	fs := flag.NewFlagSet("eval select", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rulesPath := fs.String("rules", "evals/selection.yaml", "rules that map changed paths to evals")
	base := fs.String("base", "", "take the changed paths from git diff against this commit")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	doc, err := os.ReadFile(*rulesPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval select: %v\n", err)
		return exitFailure
	}
	rules, err := evals.Parse(doc)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval select: %v\n", err)
		return exitFailure
	}
	changed, err := changedPaths(fs.Args(), *base)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval select: %v\n", err)
		return exitUsage
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

// changedPaths adds the files git reports as changed since base to the paths given.
func changedPaths(given []string, base string) ([]string, error) {
	if base == "" {
		return given, nil
	}
	if strings.HasPrefix(base, "-") {
		return nil, errors.New("--base must be a commit, not an option")
	}
	out, err := exec.CommandContext(context.Background(), "git", "diff", "--name-only", "--no-renames", "-z", base+"...HEAD", "--").Output() //nolint:gosec // G204: a fixed program; the base is an argument of git diff, not a shell word
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			given = append(given, p)
		}
	}
	return given, nil
}
