package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/ilyachakun/nightshift/internal/execution/skills"
)

// skillsCmd is `nightshift skills sync`: it vendors the skills of skills/vendor.yaml by SHA and
// prints the diff report (design section 12). Opening the pull request is the caller's job.
func skillsCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "sync" {
		_, _ = fmt.Fprintln(stderr, "usage: nightshift skills sync [--file skills/vendor.yaml] [--dir skills/vendor] [--dry-run]")
		return exitUsage
	}
	fs := flag.NewFlagSet("skills sync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "skills/vendor.yaml", "the list of external skills")
	dir := fs.String("dir", "skills/vendor", "where vendored skills are written")
	dry := fs.Bool("dry-run", false, "report only; write nothing")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	rs, err := skills.Sync(context.Background(), skills.Options{File: *file, Dir: *dir, DryRun: *dry})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift skills sync: %v\n", err)
		return exitFailure
	}
	_, _ = io.WriteString(stdout, skills.Format(rs))
	if skills.Refused(rs) {
		_, _ = fmt.Fprintln(stderr, "nightshift skills sync: nothing was written: an entry was refused")
		return exitFailure
	}
	return exitOK
}
