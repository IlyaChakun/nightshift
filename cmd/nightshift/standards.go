package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ilyachakun/nightshift/internal/assurance/standards"
)

// standardsCmd lists the standards cards and copies the base tool configuration of the stacks
// into a project checkout (design section 9). Opening the pull request is the caller's job.
func standardsCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "list" && args[0] != "sync") {
		_, _ = fmt.Fprintln(stderr, "usage: nightshift standards list|sync [--dir standards] [--stack go,python] [--repo-dir DIR]")
		return exitUsage
	}
	fs := flag.NewFlagSet("standards "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "standards", "the standards packs")
	stack := fs.String("stack", "", "comma-separated stacks or disciplines; empty means all (list) or none (sync)")
	repo := fs.String("repo-dir", "", "sync: the project's checkout")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	var folders []string
	if *stack != "" {
		folders = strings.Split(*stack, ",")
	}
	fsys := os.DirFS(*dir)
	if args[0] == "list" {
		cards, err := standards.List(fsys, folders)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "nightshift standards:", err)
			return exitFailure
		}
		for _, c := range cards {
			_, _ = fmt.Fprintf(stdout, "%-28s %-9s %-8s %-6s %s\n", c.ID, c.Enforcement, c.Status, c.Severity, c.Title)
		}
		return exitOK
	}
	if *repo == "" || len(folders) == 0 {
		_, _ = fmt.Fprintln(stderr, "nightshift standards sync: --repo-dir and --stack are required")
		return exitUsage
	}
	written, kept, err := standards.Sync(fsys, folders, *repo)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift standards:", err)
		return exitFailure
	}
	for _, f := range written {
		_, _ = fmt.Fprintln(stdout, "wrote", f)
	}
	for _, f := range kept {
		_, _ = fmt.Fprintln(stdout, "kept ", f, "(the project has its own)")
	}
	return exitOK
}
