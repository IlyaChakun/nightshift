package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
)

const taskUsage = `nightshift task: usage: task add <tenant> [--repo owner/name] "<what to do>"`

// taskCmd handles `nightshift task add`, run inside the nightshift service.
func taskCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "add" {
		_, _ = fmt.Fprintln(stderr, taskUsage)
		return exitUsage
	}
	name := args[1]
	fs := flag.NewFlagSet("task add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("config", "/etc/nightshift", "directory with instance.yaml")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with compose secrets")
	repo := fs.String("repo", "", "owner/name; default the tenant's first repository")
	if err := fs.Parse(args[2:]); err != nil {
		return exitUsage
	}
	text := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(text) == "" {
		_, _ = fmt.Fprintln(stderr, taskUsage)
		return exitUsage
	}
	if err := addTask(context.Background(), stdout, *dir, config.Secrets{Dir: *secrets}, name, *repo, text); err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift task add:", err)
		return exitFailure
	}
	return exitOK
}

func addTask(ctx context.Context, stdout io.Writer, dir string, sec config.Secrets, name, repo, text string) error {
	inst, err := config.LoadInstance(config.NewValidator(), dir)
	if err != nil {
		return err
	}
	t, ok := inst.Tenants[name]
	if !ok {
		return fmt.Errorf("instance %s has no tenant %q", inst.Name, name)
	}
	url, err := sec.Read(secretServiceURL)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	task, err := dispatch.AddTask(ctx, pool, t, repo, text, "cli")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "queued task #%d in %s: %s\n", task.ID, task.Repo, task.Title)
	return nil
}
