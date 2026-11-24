package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/harness/fake"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/runner"
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
	r := &runner.Runner{
		Jobs:     &jobsapi.Client{BaseURL: *url, WorkerToken: token},
		WorkerID: *id, Git: &gitops.Git{Dir: *repos}, WorkDir: *workDir, ArtifactDir: *artifacts,
		Harness: harnessFor(*scenario),
		Schema:  func(path string) ([]byte, error) { return schemas.FS.ReadFile(trimSchemaDir(path)) },
		Wait:    20 * time.Second, Log: log,
	}
	if err := r.Loop(ctx); err != nil {
		log.Error("work failed", "error", err)
		return exitFailure
	}
	return exitOK
}

// harnessFor maps a provider to its harness. Only the fake one exists until the claude
// adapter lands (phase 2, item 5).
func harnessFor(scenario string) func(string) (harness.Harness, error) {
	return func(provider string) (harness.Harness, error) {
		if provider != "fake" || scenario == "" {
			return nil, fmt.Errorf("work: no harness for provider %q", provider)
		}
		h, err := fake.Load(scenario)
		if err != nil {
			return nil, errors.New("work: " + err.Error())
		}
		return h, nil
	}
}

func trimSchemaDir(path string) string {
	const prefix = "schemas/"
	if len(path) > len(prefix) && path[:len(prefix)] == prefix {
		return path[len(prefix):]
	}
	return path
}
