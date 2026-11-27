package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
)

const taskUsage = `nightshift task: usage: task add <tenant> [--repo owner/name] [--not-before RFC3339] [--priority -10..10] [--allow-fallback] "<what to do>"`

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
	notBefore := fs.String("not-before", "", "do not start before this time (RFC 3339, for example 2026-11-26T22:00:00+01:00)")
	priority := fs.Int("priority", 0, "-10 to 10; a higher one goes first")
	allowFallback := fs.Bool("allow-fallback", false, "may run on a fallback provider when the primary ones are at a long limit")
	if err := fs.Parse(args[2:]); err != nil {
		return exitUsage
	}
	text := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(text) == "" {
		_, _ = fmt.Fprintln(stderr, taskUsage)
		return exitUsage
	}
	plan := dispatch.Plan{Priority: *priority, AllowFallback: *allowFallback}
	if *notBefore != "" {
		at, err := time.Parse(time.RFC3339, *notBefore)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "nightshift task add: --not-before must be an RFC 3339 time:", err)
			return exitUsage
		}
		plan.NotBefore = at
	}
	if err := addTask(context.Background(), stdout, *dir, config.Secrets{Dir: *secrets}, name, *repo, text, plan); err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift task add:", err)
		return exitFailure
	}
	return exitOK
}

func addTask(ctx context.Context, stdout io.Writer, dir string, sec config.Secrets, name, repo, text string, plan dispatch.Plan) error {
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
	task, err := dispatch.AddTask(ctx, pool, t, repo, text, "cli", plan)
	if err != nil {
		return err
	}
	when := ""
	if task.NotBefore.Valid {
		when = ", starts after " + task.NotBefore.Time.Format(time.RFC3339)
	}
	_, _ = fmt.Fprintf(stdout, "queued task #%d in %s: %s%s\n", task.ID, task.Repo, task.Title, when)
	return nil
}
