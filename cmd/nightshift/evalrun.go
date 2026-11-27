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

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/evals"
	"github.com/ilyachakun/nightshift/internal/execution/harness"
	"github.com/ilyachakun/nightshift/internal/execution/harness/claude"
	"github.com/ilyachakun/nightshift/internal/execution/harness/fake"
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
	role := fl.String("role", "builder", "builder (polygon tasks) or lead (frozen chat cases)")
	cases := fl.String("cases", "evals/lead/cases.yaml", "lead: the frozen cases")
	rolesDir := fl.String("roles", "roles", "lead: directory with roles/lead.md and roles/lead.yaml")
	skillsDir := fl.String("skills", "skills", "lead: directory with the own skills")
	fixture := fl.String("fixture", "evals/lead/repo", "lead: a small repository the cases run in (copied afresh for each case)")
	secrets := fl.String("secrets", config.DefaultSecretsDir, "claude: directory with harness/<VARIABLE> files, the same as `nightshift work --secrets`")
	if err := fl.Parse(args); err != nil {
		return exitUsage
	}
	home := ""
	if *kind == "claude" { // the CLI keeps its state in HOME; the credential itself comes through the environment
		dir, err := os.MkdirTemp("", "eval-home-")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "nightshift eval run: %v\n", err)
			return exitFailure
		}
		defer func() { _ = os.RemoveAll(dir) }()
		home = dir
	}
	h, err := evalHarness(*kind, *scenario, *auth, home)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval run: %v\n", err)
		return exitUsage
	}
	env, err := evalEnv(*kind, *auth, *secrets)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval run: eval did not run: %v\n", err)
		return exitFailure
	}
	if *candidate == "" {
		*candidate = *kind
	}
	if !safeName(*candidate) {
		_, _ = fmt.Fprintln(stderr, "nightshift eval run: --candidate must be a plain name")
		return exitUsage
	}
	if *role == "lead" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return evalRunLead(ctx, h, leadEvalOptions{Cases: *cases, RolesDir: *rolesDir, SkillsDir: *skillsDir, Fixture: *fixture, Env: env, Out: filepath.Join(*out, "lead", *candidate)}, stdout, stderr)
	}
	if *role != "builder" {
		_, _ = fmt.Fprintln(stderr, "nightshift eval run: --role must be builder or lead")
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
	r := evals.Runner{Harness: h, Candidate: *candidate, Schema: schema, Env: env}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	passed, total, err := runSet(ctx, r, v, names, *tasks, *set, filepath.Join(*out, *candidate), stdout)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift eval run: %v\n", err)
		return exitFailure
	}
	_, _ = fmt.Fprintf(stdout, "solved %d of %d tasks of the %s set\n", passed, total, *set)
	if total == 0 {
		_, _ = fmt.Fprintf(stderr, "nightshift eval run: no task in the %s set\n", *set)
		return exitFailure
	}
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

func evalHarness(kind, scenario, auth, home string) (harness.Harness, error) {
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
		return &claude.Harness{Auth: a, Home: home, PermissionMode: "acceptEdits", AllowedTools: []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash"}}, nil
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

// runSet solves the tasks of one set in name order and saves each result.
func runSet(ctx context.Context, r evals.Runner, v *config.Validator, names []string, tasks, set, out string, stdout io.Writer) (passed, total int, err error) {
	for _, name := range names {
		t, err := evals.LoadTask(filepath.Join(tasks, name))
		if err != nil {
			return passed, total, fmt.Errorf("%s: %w", name, err)
		}
		if t.Set != set {
			continue
		}
		res, err := r.Run(ctx, t)
		if err == nil {
			err = saveResult(v, out, res)
		}
		if err != nil {
			return passed, total, fmt.Errorf("%s: %w", name, err)
		}
		total++
		if res.Passed {
			passed++
		}
		_, _ = fmt.Fprintf(stdout, "%s %s\n", t.ID, map[bool]string{true: "pass", false: "fail"}[res.Passed])
	}
	return passed, total, nil
}

// evalEnv is the harness environment of an eval run, built the way `nightshift work` builds it:
// one file per credential in <secrets>/harness (the file name is the variable), plus the
// non-secret proxy and telemetry settings. The claude harness passes on only the credential of
// its auth mode, so nothing else from the directory reaches the CLI. A missing credential stops
// the run before any case is sent.
func evalEnv(kind, auth, secrets string) (map[string]string, error) {
	if kind != "claude" {
		return map[string]string{}, nil
	}
	env, err := readEnvDir(secrets, "harness")
	if err != nil {
		return nil, err
	}
	passEnv(env)
	name := "ANTHROPIC_API_KEY"
	if auth == string(claude.Subscription) {
		name = "CLAUDE_CODE_OAUTH_TOKEN"
	}
	if env[name] == "" {
		return nil, fmt.Errorf("no %s in %s: put the credential there, as on the worker (docs/secrets.md, \"harness\")", name, filepath.Join(secrets, "harness"))
	}
	return env, nil
}
