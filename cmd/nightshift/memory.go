package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/control/mask"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/knowledge/memory"
)

const memoryUsage = `nightshift memory: usage: memory show <tenant> | memory remember <tenant> "<fact>"`

// memoryCmd handles `nightshift memory`, run inside the nightshift service: the terminal door
// to the tenant's project memory page (`/ns memory` and `/ns remember` in chat).
func memoryCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[0] != "show" && args[0] != "remember") {
		_, _ = fmt.Fprintln(stderr, memoryUsage)
		return exitUsage
	}
	fs := flag.NewFlagSet("memory "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("config", "/etc/nightshift", "directory with instance.yaml")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with compose secrets")
	if err := fs.Parse(args[2:]); err != nil {
		return exitUsage
	}
	text := strings.Join(fs.Args(), " ")
	if args[0] == "remember" && strings.TrimSpace(text) == "" {
		_, _ = fmt.Fprintln(stderr, memoryUsage)
		return exitUsage
	}
	if err := memoryRun(context.Background(), stdout, *dir, config.Secrets{Dir: *secrets}, args[0], args[1], text); err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift memory:", err)
		return exitFailure
	}
	return exitOK
}

func memoryRun(ctx context.Context, stdout io.Writer, dir string, sec config.Secrets, verb, tenant, text string) error {
	inst, err := config.LoadInstance(config.NewValidator(), dir)
	if err != nil {
		return err
	}
	if _, ok := inst.Tenants[tenant]; !ok {
		return fmt.Errorf("instance %s has no tenant %q", inst.Name, tenant)
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
	store := memory.Store{Pool: pool}
	if verb == "remember" {
		// A secret pasted into a fact must not be kept in a page that goes into prompts.
		err := store.Remember(ctx, tenant, mask.NewMasker().String(text), "cli", time.Now())
		if errors.Is(err, memory.ErrStale) {
			err = store.Remember(ctx, tenant, mask.NewMasker().String(text), "cli", time.Now()) // read again once
		}
		if err != nil {
			return err
		}
	}
	page, err := store.Read(ctx, tenant)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "memory page of %s: revision %d, %d of %d entries, %d of %d bytes\n",
		tenant, page.Revision, len(page.Entries), memory.MaxEntries, memory.Size(page.Entries), memory.MaxBytes)
	for _, e := range page.Entries {
		exp := "pinned"
		if !e.ExpiresAt.IsZero() {
			exp = "until " + e.ExpiresAt.Format(time.DateOnly)
		}
		_, _ = fmt.Fprintf(stdout, "- %s: %s (%s, %s)\n", e.Key, e.Text, e.Source, exp)
	}
	return nil
}
