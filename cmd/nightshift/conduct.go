package main

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/policy"
	"github.com/ilyachakun/nightshift/internal/settings"
	"github.com/ilyachakun/nightshift/internal/train"
)

const secretGitHubToken = "github_token"

// conductorDirs says where the conductor keeps its clones and finds what the worker wrote
// and what roles and standards it needs. An empty Repos turns the conductor off.
type conductorDirs struct {
	Repos, Artifacts, Roles, Standards string
	Outbox                             *dispatch.Outbox // the Lead stage speaks through it
}

// startConductor runs the conductor loop for every tenant. It stays off without a clone
// directory or without a GitHub token, so a service started for chat alone is unchanged.
func startConductor(ctx context.Context, wg *sync.WaitGroup, log *slog.Logger, pool *pgxpool.Pool, inst *config.Instance, sec config.Secrets, d conductorDirs) error {
	if d.Repos == "" {
		return nil
	}
	access, err := githubAccess(sec)
	if err != nil {
		return err
	}
	if access == nil {
		log.InfoContext(ctx, "conductor is off: no GitHub credential", "secrets", []string{secretGitHubAppKey, secretGitHubToken})
		return nil
	}
	roles, err := config.LoadRoles(os.DirFS(d.Roles))
	if err != nil {
		return err
	}
	cons, err := policy.NewConstitution(nil)
	if err != nil {
		return err
	}
	forge := access.forge
	val := config.NewValidator()
	git := &gitops.Git{Dir: d.Repos, Guard: cons.Check}
	store := &settings.Store{Pool: pool}
	// One conductor per tenant for the life of the service: its router remembers which
	// providers hit a limit.
	var mu sync.Mutex
	made := map[string]*dispatch.Conductor{}
	build := func(t config.Tenant) *dispatch.Conductor {
		mu.Lock()
		defer mu.Unlock()
		if c, ok := made[t.Name]; ok {
			return c
		}
		c := &dispatch.Conductor{
			Pool: pool, Mover: &flow.Mover{Pool: pool}, Roles: roles, Git: git, Forge: forge,
			Router: &dispatch.Router{Pool: pool, Providers: inst.Providers, Chains: t.Providers},
			Remote: func(repo string) string { return "https://github.com/" + repo + ".git" },
			Auth:   access.git, ArtifactDir: d.Artifacts,
			Identity:  gitops.Identity{Name: "nightshift", Email: "nightshift@users.noreply.github.com"},
			Validator: val, Roll: rollDice, Standards: os.DirFS(d.Standards), Outbox: d.Outbox, Log: log,
		}
		c.Train = &train.Train{Journal: train.BookJournal{Q: bookdb.New(pool)}, Git: c, Forge: forge, Mover: c.Mover, Paused: store.IsPaused}
		made[t.Name] = c
		return c
	}
	tk := &dispatch.Ticker{Pool: pool, Tenants: inst.Tenants, For: build, Every: 15 * time.Second, Log: log}
	wg.Go(func() { tk.Run(ctx) })
	return nil
}

// rollDice draws the number that decides review sampling; it is calibration, not security.
func rollDice() float64 { return rand.Float64() } //nolint:gosec // sampling of reviews, not a secret
