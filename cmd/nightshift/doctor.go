package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/adapters"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/onboard"
)

// doctorCmd checks the setup of an instance from inside a container: `nightshift doctor` in
// the nightshift service, `nightshift doctor --worker` in the worker, because each holds
// only its own secrets.
func doctorCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	worker := fs.Bool("worker", false, "check the worker: agent CLIs, model credentials, egress")
	dir := fs.String("config", "/etc/nightshift", "directory with instance.yaml")
	secrets := fs.String("secrets", "", "directory with compose secrets; default of the service")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	sec := config.Secrets{Dir: *secrets}
	var checks []onboard.Check
	if *worker {
		if sec.Dir == "" {
			sec.Dir = "/run/worker-secrets"
		}
		env := map[string]string{}
		passEnv(env)
		checks = workerChecks(sec, env["HTTPS_PROXY"], exec.LookPath)
	} else {
		var closeBook func()
		checks, closeBook = serviceChecks(*dir, sec)
		defer closeBook()
	}
	if !onboard.RunChecks(context.Background(), stdout, checks) {
		return exitFailure
	}
	return exitOK
}

// serviceChecks are the checks of the nightshift service. The returned function closes the
// book connection a check opened.
func serviceChecks(dir string, sec config.Secrets) ([]onboard.Check, func()) {
	inst, loadErr := config.LoadInstance(config.NewValidator(), dir)
	book, closeBook := bookCheck(sec)
	checks := []onboard.Check{
		{Name: "config", Fix: "docs/setup.md section 6", Run: func(context.Context) (string, error) {
			if loadErr != nil {
				return "", loadErr
			}
			return fmt.Sprintf("instance %s, %d tenant(s)", inst.Name, len(inst.Tenants)), nil
		}},
		book,
	}
	if loadErr != nil {
		return checks, closeBook
	}
	access, accessErr := githubAccessFor(sec, inst.Tenants)
	checks = append(checks, onboard.Check{Name: "code host credential", Fix: "docs/setup.md section 5", Run: func(context.Context) (string, error) {
		switch {
		case accessErr != nil:
			return "", accessErr
		case access == nil:
			return "", errors.New("no GitHub App and no token: the conductor is off")
		}
		return access.kind, nil
	}})
	for _, name := range slices.Sorted(maps.Keys(inst.Tenants)) {
		t := inst.Tenants[name]
		checks = append(checks, tenantChecks(inst, t, sec)...)
		if access == nil {
			continue
		}
		if reason, off := access.off[name]; off {
			checks = append(checks, onboard.Check{Name: name + " code host credential", Fix: "docs/secrets.md: tenants/<name>/github_token", Run: func(context.Context) (string, error) {
				return "", reason
			}})
			continue
		}
		checks = append(checks, repositoryChecks(t, access.forge)...)
	}
	return checks, closeBook
}

func bookCheck(sec config.Secrets) (onboard.Check, func()) {
	var pool *pgxpool.Pool
	check := onboard.Check{Name: "book", Fix: "docs/setup.md sections 1 and 7: init-secrets.sh, then docker compose up", Run: func(ctx context.Context) (string, error) {
		url, err := sec.Read(secretServiceURL)
		if err != nil {
			return "", err
		}
		if pool, err = pgxpool.New(ctx, url); err != nil {
			return "", errors.New("cannot connect")
		}
		var tasks int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM book.tasks").Scan(&tasks); err != nil {
			return "", fmt.Errorf("not reachable or not migrated: %w", err)
		}
		return fmt.Sprintf("migrated, %d task(s)", tasks), nil
	}}
	return check, func() {
		if pool != nil {
			pool.Close()
		}
	}
}

// workflows counts the CI workflows of a repository the credential can reach.
type workflows interface {
	Workflows(ctx context.Context, repo string) (int, error)
}

func repositoryChecks(t config.Tenant, host workflows) []onboard.Check {
	checks := make([]onboard.Check, 0, len(t.Repos))
	for _, full := range t.Repos {
		_, repo, _ := strings.Cut(full, "/")
		checks = append(checks, onboard.Check{
			Name: t.Name + " repository " + repo, Fix: "docs/setup.md sections 4 and 5: install the app on it and give it a CI check on pull requests",
			Run: func(ctx context.Context) (string, error) {
				n, err := host.Workflows(ctx, repo)
				if err != nil {
					return "", err
				}
				if n == 0 {
					return "", errors.New("no CI workflow: a task would wait in checks forever")
				}
				return fmt.Sprintf("reachable, %d workflow(s)", n), nil
			},
		})
	}
	return checks
}

// tenantChecks are the parts of a tenant that need no code host: its providers and its chat.
func tenantChecks(inst *config.Instance, t config.Tenant, sec config.Secrets) []onboard.Check {
	return []onboard.Check{
		{Name: t.Name + " builder provider", Fix: "docs/setup.md section 6: providers.yaml and the tenant's providers", Run: func(context.Context) (string, error) {
			chain := t.Providers["builder"]
			if len(chain) == 0 {
				return "", errors.New("the tenant names no provider for builder")
			}
			for _, p := range chain {
				if _, ok := inst.Providers[p]; !ok {
					return "", fmt.Errorf("providers.yaml has no %q", p)
				}
			}
			return strings.Join(chain, ", "), nil
		}},
		{Name: t.Name + " chat", Fix: "docs/setup.md section 3: tokens, the owner's member id and channel ids", Run: func(ctx context.Context) (string, error) {
			if t.Chat.Adapter == "inbox" { // lives in the book, which has its own check
				return "inbox, cards wait in the book", nil
			}
			chat, _, err := adapters.NewChat(t.Chat.Adapter, sec, slog.New(slog.DiscardHandler))
			if err != nil {
				return "", err
			}
			prober, ok := chat.(interface {
				Probe(ctx context.Context) (string, error)
			})
			if !ok {
				return t.Chat.Adapter + " adapter, nothing to sign in to", nil
			}
			if t.People.Owner.Chat == "" || len(t.Chat.Channels) == 0 {
				return "", errors.New("the owner's chat id or the channels are missing: the bot would ignore every message")
			}
			return prober.Probe(ctx)
		}},
	}
}

// workerChecks look at what only the worker has: the agent CLIs, the model credentials and the
// way out through the proxy. look finds an executable.
func workerChecks(sec config.Secrets, proxy string, look func(string) (string, error)) []onboard.Check {
	return []onboard.Check{
		{Name: "agent CLIs", Fix: "docs/setup.md section 6: set the versions in deploy/.env and rebuild the worker", Run: func(context.Context) (string, error) {
			var found []string
			for _, bin := range []string{"claude", "codex"} {
				if _, err := look(bin); err == nil {
					found = append(found, bin)
				}
			}
			if len(found) == 0 {
				return "", errors.New("neither claude nor codex is in the image")
			}
			return strings.Join(found, ", "), nil
		}},
		{Name: "model credentials", Fix: "docs/setup.md section 2: one file per credential in deploy/secrets/harness", Run: func(context.Context) (string, error) {
			return credentials(sec.Dir)
		}},
		{Name: "egress proxy", Fix: "docs/runbook.md: the egress service must run and HTTPS_PROXY must name it", Run: func(ctx context.Context) (string, error) {
			if proxy == "" {
				return "", errors.New("HTTPS_PROXY is not set")
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxy+"/healthcheck", nil)
			if err != nil {
				return "", err
			}
			resp, err := (&http.Client{Transport: &http.Transport{}}).Do(req)
			if err != nil {
				return "", fmt.Errorf("the proxy does not answer: %w", err)
			}
			_ = resp.Body.Close()
			return "answers at " + proxy, nil
		}},
	}
}

// credentials names the credential files that hold something; it never reads them into the
// report.
func credentials(secrets string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(secrets, "harness"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	var found []string
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() && info.Size() > 0 {
			found = append(found, e.Name())
		}
	}
	if len(found) == 0 {
		return "", errors.New("no model credential")
	}
	return strings.Join(found, ", "), nil
}
