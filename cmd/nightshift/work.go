package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/harness/claude"
	"github.com/ilyachakun/nightshift/internal/harness/codex"
	"github.com/ilyachakun/nightshift/internal/harness/fake"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/runner"
	"github.com/ilyachakun/nightshift/internal/stack"
	"github.com/ilyachakun/nightshift/schemas"
)

func work(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("work", flag.ContinueOnError)
	fs.SetOutput(stderr)
	url := fs.String("jobs-url", "http://nightshift:8081", "base URL of the jobs API")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with compose secrets")
	id := fs.String("worker-id", "worker-1", "name of this worker in the book")
	repos := fs.String("repos", "/repos", "repository mirrors (read-only)")
	workDir := fs.String("work", "/work", "task checkouts")
	artifacts := fs.String("artifacts", "/artifacts", "job artifacts")
	rolesDir := fs.String("roles", "/roles", "role instructions (roles/<name>.md)")
	skillsDir := fs.String("skills", "/skills", "skills: own/ and vendor/")
	stacksDir := fs.String("stacks", "/stacks", "stack packs (stacks/<name>/pack.yaml); missing means no stack tools")
	agentUID := fs.Int("agent-uid", 0, "run the agent and project commands as this uid; 0 keeps the current user")
	agentGID := fs.Int("agent-gid", 0, "group of the agent user")
	scenario := fs.String("fake-scenario", "", "scenario file for the fake harness (tests and demos)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	token, err := (config.Secrets{Dir: *secrets}).Read(secretWorkerToken)
	if err != nil {
		log.Error("work failed", "error", err)
		return exitFailure
	}
	env, err := readEnvDir(*secrets, "harness")
	if err != nil {
		log.Error("work failed", "error", err)
		return exitFailure
	}
	packs, err := loadStacks(*stacksDir)
	if err != nil {
		log.Error("work failed", "error", err)
		return exitFailure
	}
	r := &runner.Runner{
		Stacks: packs,
		Env:    env, AgentUID: *agentUID, AgentGID: *agentGID, AgentHome: agentHome(*agentUID),
		Jobs:     &jobsapi.Client{BaseURL: *url, WorkerToken: token},
		WorkerID: *id, Git: &gitops.Git{Dir: *repos}, WorkDir: *workDir, ArtifactDir: *artifacts, RolesDir: *rolesDir, SkillsDir: *skillsDir,
		Harness: harnessFor(*scenario, uint32(*agentUID), uint32(*agentGID)), //nolint:gosec // a uid and gid fit in 32 bits,
		Schema:  func(path string) ([]byte, error) { return schemas.FS.ReadFile(trimSchemaDir(path)) },
		Wait:    20 * time.Second, Log: log,
	}
	if err := r.Loop(ctx); err != nil {
		log.Error("work failed", "error", err)
		return exitFailure
	}
	return exitOK
}

// harnessFor maps a job's harness to an adapter. Roles keep the same explicit tool list
// until role files carry their own (phase 3).
func harnessFor(scenario string, uid, gid uint32) func(jobsapi.Job) (harness.Harness, error) {
	return func(job jobsapi.Job) (harness.Harness, error) {
		if job.Harness == "claude" {
			return &claude.Harness{
				Auth: claude.Auth(job.Auth), AgentUID: uid, AgentGID: gid,
				PermissionMode: "acceptEdits", AllowedTools: []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash"},
			}, nil
		}
		if job.Harness == "codex" {
			if job.Auth != "api_key" {
				return nil, fmt.Errorf("work: codex supports auth api_key only, provider %q has %q", job.Provider, job.Auth)
			}
			// The container is the boundary (ADR 0004); the CLI's own sandbox needs namespaces it lacks.
			return &codex.Harness{
				Model: job.Model, BaseURL: job.BaseURL, KeyEnv: job.KeyEnv, Sandbox: "danger-full-access",
				Home: "/home/agent", AgentUID: uid, AgentGID: gid,
			}, nil
		}
		if job.Harness != "fake" || scenario == "" {
			return nil, fmt.Errorf("work: no harness %q for provider %q", job.Harness, job.Provider)
		}
		h, err := fake.Load(scenario)
		if err != nil {
			return nil, errors.New("work: " + err.Error())
		}
		return h, nil
	}
}

// readEnvDir reads <secrets>/<name>/: one file per credential, the file name is the variable.
// A missing directory means no credentials.
func readEnvDir(secrets, name string) (map[string]string, error) {
	dir := filepath.Join(secrets, name)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work: %w", err)
	}
	env := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // the secrets directory is configuration
		if err != nil {
			return nil, fmt.Errorf("work: %w", err)
		}
		env[e.Name()] = strings.TrimSpace(string(raw))
	}
	return env, nil
}

func trimSchemaDir(path string) string {
	const prefix = "schemas/"
	if len(path) > len(prefix) && path[:len(prefix)] == prefix {
		return path[len(prefix):]
	}
	return path
}

// agentHome is the HOME of the agent user, where the project's verify command runs. Without
// a separate agent user the worker runs no repository commands.
func agentHome(uid int) string {
	if uid == 0 {
		return ""
	}
	return "/home/agent"
}

// loadStacks reads the stack packs; a missing directory means none, so a worker started
// without them runs only the project's verify command.
func loadStacks(dir string) (stack.Packs, error) {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return stack.Load(os.DirFS(dir))
}
