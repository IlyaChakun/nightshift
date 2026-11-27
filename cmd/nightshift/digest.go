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
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/observability/summary"
)

const digestUsage = `nightshift digest: usage: digest <tenant> [--since 24h] [--send]`

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
	send := fs.Bool("send", false, "also queue the digest as a message to the owner (the running service delivers it)")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	if *window <= 0 {
		_, _ = fmt.Fprintln(stderr, digestUsage)
		return exitUsage
	}
	if err := digestRun(context.Background(), stdout, *dir, config.Secrets{Dir: *secrets}, args[0], *window, *send); err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift digest:", err)
		return exitFailure
	}
	return exitOK
}

func digestRun(ctx context.Context, stdout io.Writer, dir string, sec config.Secrets, name string, window time.Duration, send bool) error {
	inst, err := config.LoadInstance(config.NewValidator(), dir)
	if err != nil {
		return err
	}
	t, ok := inst.Tenants[name]
	if !ok {
		return fmt.Errorf("instance %s has no tenant %q", inst.Name, name)
	}
	acc, err := githubAccessFor(sec, map[string]config.Tenant{name: t})
	if err != nil {
		return err
	}
	if acc == nil {
		return fmt.Errorf("no GitHub App or token in %s, so merges cannot be listed", sec.Dir)
	}
	if reason, off := acc.off[name]; off {
		return fmt.Errorf("tenant %s has no usable GitHub credential: %w", name, reason)
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
	now, err := digestNow(t.Schedule.Timezone, time.Now())
	if err != nil {
		return fmt.Errorf("timezone of %s: %w", name, err)
	}
	d, err := summary.BuildDigest(ctx, pool, acc.forge, name, repos, now.Add(-window), now)
	if err != nil {
		return err
	}
	d.Health = summary.BuildHealth(ctx, acc.forge, repos, time.Now())
	d.Plan = summary.BuildPlan(ctx, acc.forge, repos)
	_, _ = fmt.Fprintln(stdout, d.Format())
	if !send {
		return nil
	}
	if d.MergeErr != nil {
		return fmt.Errorf("not sent: the merge list is incomplete (%w); fix the code host access and run it again", d.MergeErr)
	}
	return sendDigest(ctx, stdout, pool, url, t, d, now)
}

// digestNow is the end of the digest window in the tenant's own zone, as in the daily digest: the
// owner reads the times, not the server. The digest prints its times in the zone of its window.
func digestNow(timezone string, now time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	return now.In(loc), nil
}

// repoOwnerName drops the host from a repository in tenant.yaml (host/owner/name).
func repoOwnerName(repo string) string {
	if _, name, ok := strings.Cut(repo, "/"); ok && strings.Contains(name, "/") {
		return name
	}
	return repo
}

// sendDigest queues the digest for the owner under a key of its own, so it goes out even when
// the daily one already did. The service sends it; without a running service it waits in the outbox.
func sendDigest(ctx context.Context, stdout io.Writer, pool *pgxpool.Pool, serviceURL string, t config.Tenant, d summary.Digest, now time.Time) error {
	if !canMessageOwner(t) || t.People.Owner.Chat == "" {
		return fmt.Errorf("tenant %s cannot be messaged without a task: it needs the slack-app chat adapter and people.owner.chat", t.Name)
	}
	m := &dispatch.Morning{Pool: pool, Outbox: &dispatch.Outbox{Pool: pool, Mask: dispatch.NewMasker(serviceURL)}}
	if err := m.Queue(ctx, t, d, "digest-resend:"+t.Name+":"+now.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "Queued for the owner; the running service delivers it within seconds.")
	return nil
}
