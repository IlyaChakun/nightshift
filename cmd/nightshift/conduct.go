package main

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/policy"
	"github.com/ilyachakun/nightshift/internal/settings"
	"github.com/ilyachakun/nightshift/internal/train"
)

const secretGitHubToken = "github_token"

// conductorDirs says where the conductor keeps its clones and finds what the worker wrote
// and what roles and standards it needs. An empty Repos turns the conductor off.
type conductorDirs struct {
	Repos, Artifacts, Roles, Standards string
	DeployerURL                        string           // the deploy service; a release target requires it
	Outbox                             *dispatch.Outbox // the Lead stage speaks through it
	Trackers                           map[string]integration.Tracker
}

// startConductor runs the conductor loop for every tenant with a GitHub credential. It stays off without a clone
// directory or without a GitHub credential, so a service started for chat alone is unchanged.
func startConductor(ctx context.Context, wg *sync.WaitGroup, log *slog.Logger, pool *pgxpool.Pool, inst *config.Instance, sec config.Secrets, d conductorDirs) error {
	if d.Repos == "" {
		return nil
	}
	access, err := githubAccessFor(sec, inst.Tenants)
	if err != nil {
		return err
	}
	if access == nil {
		log.InfoContext(ctx, "conductor is off: no GitHub credential", "secrets", []string{secretGitHubAppKey, secretGitHubToken})
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(access.off)) {
		log.WarnContext(ctx, "conductor is off for a tenant: no usable GitHub credential", "tenant", name, "reason", access.off[name])
	}
	tenants := access.on(inst.Tenants)
	if len(tenants) == 0 {
		log.InfoContext(ctx, "conductor is off: no tenant has a GitHub credential")
		return nil
	}
	var deployToken string
	if d.DeployerURL != "" {
		tok, err := sec.Read(secretDeployerToken)
		if err != nil {
			return err
		}
		deployToken = strings.TrimSpace(tok)
	}
	roles, err := config.LoadRoles(os.DirFS(d.Roles))
	if err != nil {
		return err
	}
	if err := checkRoleRuns(inst.Tenants, roles); err != nil {
		return err
	}
	forge := access.forge
	val := config.NewValidator()
	git := &gitops.Git{Dir: d.Repos}
	git.Guard = policy.ProjectGuard(git.Show)
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
			Router:       &dispatch.Router{Pool: pool, Providers: inst.Providers, CreditPools: inst.Pools, Chains: t.Providers, MaxWait: maxWait(t)},
			ChatLanguage: t.Language.Chat, Fallback: t.OnLimit.Fallback, Run: t.Budget.Run, RoleRuns: t.Roles,
			Remote: func(repo string) string { return "https://github.com/" + repo + ".git" },
			Auth:   access.git, ArtifactDir: d.Artifacts,
			Identity:  gitops.Identity{Name: "nightshift", Email: "nightshift@users.noreply.github.com"},
			Validator: val, Roll: rollDice, Standards: os.DirFS(d.Standards), Outbox: d.Outbox, Log: log,
		}
		c.Train = &train.Train{Journal: train.BookJournal{Q: bookdb.New(pool)}, Git: c, Forge: forge, Mover: c.Mover, Paused: store.IsPaused}
		(&deployWire{url: d.DeployerURL, token: deployToken, q: bookdb.New(pool), c: c, tenant: t}).install()
		made[t.Name] = c
		return c
	}
	tk := &dispatch.Ticker{
		Pool: pool, Tenants: tenants, For: build, Every: 15 * time.Second, Log: log,
		Projector: &dispatch.Projector{
			Pool: pool, Outbox: d.Outbox, Mover: &flow.Mover{Pool: pool}, Log: log,
			Tracker: func(t config.Tenant) integration.Tracker { return d.Trackers[t.Name] },
		},
		Health: &dispatch.HealthTasks{Pool: pool, Mover: &flow.Mover{Pool: pool}, Source: forge, Log: log},
		Morning: &dispatch.Morning{
			Pool: pool, Outbox: d.Outbox, Merges: forge, Health: forge, Plan: forge, Log: log,
			CanMessageOwner: canMessageOwner,
		},
	}
	wg.Go(func() { tk.Run(ctx) })
	return nil
}

// rollDice draws the number that decides review sampling; it is calibration, not security.
func rollDice() float64 { return rand.Float64() } //nolint:gosec // sampling of reviews, not a secret

// maxWait reads the tenant's on_limit.max_wait ("90m", "2h"); zero lets the router use its default.
func maxWait(t config.Tenant) time.Duration {
	d, err := time.ParseDuration(t.OnLimit.MaxWait)
	if err != nil {
		return 0
	}
	return d
}

// checkRoleRuns refuses a tenant's roles entry for a role that does not exist: a typo would
// otherwise leave the role on its default model without a word (ADR 0020).
func checkRoleRuns(tenants map[string]config.Tenant, roles map[string]config.Role) error {
	for name, t := range tenants {
		for role := range t.Roles {
			if _, ok := roles[role]; !ok {
				return fmt.Errorf("tenant %s: roles.%s names no role in the roles directory", name, role)
			}
		}
	}
	return nil
}

// canMessageOwner says whether the tenant's chat adapter can post to the owner without a task
// thread, which the morning digest needs.
func canMessageOwner(t config.Tenant) bool { return t.Chat.Adapter == "slack-app" }
