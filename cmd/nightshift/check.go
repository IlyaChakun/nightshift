package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ilyachakun/nightshift/internal/assurance/check"
	"github.com/ilyachakun/nightshift/internal/assurance/rubric"
	"github.com/ilyachakun/nightshift/internal/execution/harness"
	"github.com/ilyachakun/nightshift/internal/execution/harness/claude"
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
	standards := fs.String("standards", "", "standards directory; with --judge-auth it turns on the L4 rubrics")
	judgeAuth := fs.String("judge-auth", "", "claude auth for the rubric judge: subscription or api_key")
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
		opts.Docs = check.DefaultDocKinds()
	}
	if *standards != "" || *judgeAuth != "" {
		j, cards, err := rubricJudge(*standards, *judgeAuth)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "nightshift check:", err)
			return exitUsage
		}
		opts.Judge, opts.Cards = j, cards
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

// rubricJudge builds the L4 judge: a read-only Claude turn without tools per question.
func rubricJudge(standards, auth string) (rubric.Judge, func(string) ([]rubric.Card, error), error) {
	a := claude.Auth(auth)
	if standards == "" || (a != claude.Subscription && a != claude.APIKey) {
		return nil, nil, errors.New("--standards and --judge-auth (subscription or api_key) go together")
	}
	fsys := os.DirFS(standards)
	cards := func(kind string) ([]rubric.Card, error) { return rubric.Load(fsys, "industry/docs/rubrics", kind) }
	// The document is untrusted text: the turn gets no tool at all, and only its own credential.
	h := &claude.Harness{Auth: a, PermissionMode: "plan", Home: os.Getenv("HOME"), //nolint:forbidigo // the CLI keeps its state in HOME
		DisallowedTools: []string{"Read", "Grep", "Glob", "Bash", "Edit", "Write", "WebFetch", "WebSearch", "Task", "NotebookEdit"}}
	env := map[string]string{}
	for _, k := range []string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "HTTPS_PROXY", "NO_PROXY"} {
		if v := os.Getenv(k); v != "" { //nolint:forbidigo // the judge's credential and proxy come from the caller's environment
			env[k] = v
		}
	}
	return rubric.HarnessJudge{Harness: h, Budget: harness.Budget{Minutes: 5}, Env: env}, cards, nil
}
