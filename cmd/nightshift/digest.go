package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/summary"
)

const digestUsage = `nightshift digest: usage: digest <tenant> [--since 24h]`

// digestCmd prints the morning digest of a tenant: the summary, every merge and every decision
// of the window, each merge with its reason and a revert link (ADR 0013).
func digestCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		_, _ = fmt.Fprintln(stderr, digestUsage)
		return exitUsage
	}
	fs := flag.NewFlagSet("digest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("config", "/etc/nightshift", "directory with instance.yaml")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with compose secrets")
	window := fs.Duration("since", 24*time.Hour, "how far back the digest looks")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	if *window <= 0 {
		_, _ = fmt.Fprintln(stderr, digestUsage)
		return exitUsage
	}
	if err := digestRun(context.Background(), stdout, *dir, config.Secrets{Dir: *secrets}, args[0], *window); err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift digest:", err)
		return exitFailure
	}
	return exitOK
}

func digestRun(ctx context.Context, stdout io.Writer, dir string, sec config.Secrets, name string, window time.Duration) error {
	inst, err := config.LoadInstance(config.NewValidator(), dir)
	if err != nil {
		return err
	}
	t, ok := inst.Tenants[name]
	if !ok {
		return fmt.Errorf("instance %s has no tenant %q", inst.Name, name)
	}
	acc, err := githubAccess(sec)
	if err != nil {
		return err
	}
	if acc == nil {
		return fmt.Errorf("no GitHub App or token in %s, so merges cannot be listed", sec.Dir)
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
	repos := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		repos = append(repos, repoOwnerName(r))
	}
	d, err := summary.BuildDigest(ctx, pool, acc.forge, name, repos, time.Now().Add(-window))
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, d.Format())
	return nil
}

// repoOwnerName drops the host from a repository in tenant.yaml (host/owner/name).
func repoOwnerName(repo string) string {
	if _, name, ok := strings.Cut(repo, "/"); ok && strings.Contains(name, "/") {
		return name
	}
	return repo
}
