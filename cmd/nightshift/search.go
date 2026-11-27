package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/knowledge/search"
)

const searchUsage = `nightshift search: usage: search <tenant> [--kind task,decision] [--limit N] "<words>"`

// searchCmd handles `nightshift search`, run inside the nightshift service: full-text search over
// the tenant's book documents (design section 13). Lead gets the same hits in its prompt.
func searchCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, searchUsage)
		return exitUsage
	}
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("config", "/etc/nightshift", "directory with instance.yaml")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with compose secrets")
	kinds := fs.String("kind", "", "comma-separated document kinds; empty means every kind")
	limit := fs.Int("limit", 10, "most hits to show")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	text := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(text) == "" {
		_, _ = fmt.Fprintln(stderr, searchUsage)
		return exitUsage
	}
	q := search.Query{Tenant: args[0], Text: text, Visible: []string{"all", "lead_verifier"}, Limit: *limit}
	if *kinds != "" {
		q.Kinds = strings.Split(*kinds, ",")
	}
	if err := searchRun(context.Background(), stdout, *dir, config.Secrets{Dir: *secrets}, q); err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift search:", err)
		return exitFailure
	}
	return exitOK
}

func searchRun(ctx context.Context, stdout io.Writer, dir string, sec config.Secrets, q search.Query) error {
	inst, err := config.LoadInstance(config.NewValidator(), dir)
	if err != nil {
		return err
	}
	if _, ok := inst.Tenants[q.Tenant]; !ok {
		return fmt.Errorf("instance %s has no tenant %q", inst.Name, q.Tenant)
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
	hits, err := (&search.Index{Pool: pool}).Search(ctx, q)
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		_, _ = fmt.Fprintln(stdout, "no hits")
		return nil
	}
	for _, h := range hits {
		mark := ""
		if h.Status == "proposed" {
			mark = " (unverified)"
		}
		_, _ = fmt.Fprintf(stdout, "[%s] %s%s\n  %s, %s\n  %s\n", h.Kind, plain(h.Title), mark, plain(h.Source), h.QuotedAt.Format("2006-01-02"), plain(h.Quote))
	}
	return nil
}

// plain makes a stored text one line without control characters, so a document cannot write
// escape sequences into the operator's terminal.
func plain(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
