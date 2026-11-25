package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ilyachakun/nightshift/internal/check"
)

// toolFlags collects the repeatable --tool flag as name=command.
type toolFlags []check.Tool

func (t *toolFlags) String() string { return fmt.Sprint(len(*t)) }

func (t *toolFlags) Set(v string) error {
	name, command, ok := strings.Cut(v, "=")
	if !ok || name == "" || command == "" {
		return fmt.Errorf("want name=command, got %q", v)
	}
	*t = append(*t, check.Tool{Name: name, Command: command, Category: "code"})
	return nil
}

// checkCmd runs the project's verify command and the factory tools in a worktree, writes
// the reports and findings.json to --out, and fails when a new violation blocks.
func checkCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", ".", "worktree to check")
	out := fs.String("out", "", "directory for reports, logs and findings.json (required)")
	verify := fs.String("verify", "", "the project's own verify command")
	baseline := fs.String("baseline", "", "findings.json of main; only findings it lacks block")
	timeout := fs.Duration("timeout", 15*time.Minute, "limit for each command")
	docs := fs.Bool("docs", true, "lint ADRs and PRDs (doc-lint)")
	var tools toolFlags
	fs.Var(&tools, "tool", "name=command that writes SARIF to $NIGHTSHIFT_SARIF (repeatable)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *out == "" {
		_, _ = fmt.Fprintln(stderr, "nightshift check: --out is required")
		return exitUsage
	}
	var base []check.Finding
	if *baseline != "" {
		if err := readJSON(*baseline, &base); err != nil {
			_, _ = fmt.Fprintln(stderr, "nightshift check:", err)
			return exitFailure
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	opts := check.Options{Dir: *dir, Out: *out, Verify: *verify, Tools: tools, Baseline: base, Timeout: *timeout}
	if *docs {
		opts.Docs = check.DefaultDocKinds
	}
	rep, err := check.Run(ctx, opts)
	if err == nil {
		err = writeJSON(filepath.Join(*out, "findings.json"), rep.All)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift check:", err)
		return exitFailure
	}
	for _, f := range rep.New {
		_, _ = fmt.Fprintf(stdout, "%s %s: %s\n", f.Severity, f.RuleID, firstLine(f.Message))
	}
	if check.Blocking(rep.New) {
		return exitFailure
	}
	return exitOK
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path) //nolint:gosec // G304: the path is the --baseline flag from the runner
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}
