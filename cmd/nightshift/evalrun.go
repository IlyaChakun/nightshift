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
	"sort"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/evals"
	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/harness/claude"
	"github.com/ilyachakun/nightshift/internal/harness/fake"
	"github.com/ilyachakun/nightshift/schemas"
)

// evalRun is `nightshift eval run`: every task of the chosen set gets a throwaway local git
// repository, the harness solves it, the hidden tests grade it, and the repository is removed.
// It needs no GitHub, Slack, tracker or Postgres. Results go to <out>/<candidate>/<task>.json.
func evalRun(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("eval run", flag.ContinueOnError)
	fl.SetOutput(stderr)
	tasks := fl.String("tasks", "evals/tasks", "directory with one subdirectory per task")
	set := fl.String("set", "fast", "run the tasks of this set (smoke, fast, full, holdout)")
	kind := fl.String("harness", "fake", "fake or claude")
	scenario := fl.String("scenario", "", "scenario file for the fake harness")
	auth := fl.String("auth", "", "claude auth: subscription or api_key")
	candidate := fl.String("candidate", "", "name of what is under test (default: the harness)")
	out := fl.String("out", "eval-runs", "where results are written")
	if err := fl.Parse(args); err != nil {
		return exitUsage
	}
	h, err := evalHarness(*kind, *scenario, *auth)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval run: %v\n", err)
		return exitUsage
	}
	if *candidate == "" {
		*candidate = *kind
	}
	if !safeName(*candidate) {
		_, _ = fmt.Fprintln(stderr, "nightshift eval run: --candidate must be a plain name")
		return exitUsage
	}
	schema, err := builderSchema()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval run: %v\n", err)
		return exitFailure
	}
	entries, err := os.ReadDir(*tasks)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval run: %v\n", err)
		return exitFailure
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	v := config.NewValidator()
	r := evals.Runner{Harness: h, Candidate: *candidate, Schema: schema}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var passed, total int
	for _, name := range names {
		t, err := evals.LoadTask(filepath.Join(*tasks, name))
		if err == nil && t.Set == *set {
			var res evals.Result
			if res, err = r.Run(ctx, t); err == nil {
				err = saveResult(v, filepath.Join(*out, *candidate), res)
				total++
				if res.Passed {
					passed++
				}
				_, _ = fmt.Fprintf(stdout, "%s %s\n", t.ID, map[bool]string{true: "pass", false: "fail"}[res.Passed])
			}
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "nightshift eval run: %s: %v\n", name, err)
			return exitFailure
		}
	}
	_, _ = fmt.Fprintf(stdout, "solved %d of %d tasks of the %s set\n", passed, total, *set)
	if passed < total {
		return exitFailure
	}
	return exitOK
}

func builderSchema() (json.RawMessage, error) {
	raw, err := schemas.FS.ReadFile("builder-result.json")
	if err != nil {
		return nil, err
	}
	return schemas.Standalone(raw)
}

func safeName(s string) bool {
	return s != "" && s != "." && s != ".." && filepath.Base(s) == s
}

func evalHarness(kind, scenario, auth string) (harness.Harness, error) {
	switch kind {
	case "fake":
		if scenario == "" {
			return nil, errors.New("--scenario is required for the fake harness")
		}
		return fake.Load(scenario)
	case "claude":
		a := claude.Auth(auth)
		if a != claude.Subscription && a != claude.APIKey {
			return nil, errors.New("--auth must be subscription or api_key")
		}
		return &claude.Harness{Auth: a, PermissionMode: "acceptEdits", AllowedTools: []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash"}}, nil
	}
	return nil, fmt.Errorf("unknown harness %q", kind)
}

// saveResult validates a result against schemas/eval-result.json and writes it.
func saveResult(v *config.Validator, dir string, res evals.Result) error {
	doc, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	if err := v.ValidateJSON(config.KindEvalResult, doc); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, res.TaskID+".json"), append(doc, '\n'), 0o600)
}
