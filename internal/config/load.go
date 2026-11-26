package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Instance is an instance.yaml with its providers and the tenant packs it serves.
// Only the fields the core reads are typed; the schemas check the whole file.
type Instance struct {
	Name        string              `yaml:"name"`
	TrustDomain string              `yaml:"trust_domain"`
	Search      Search              `yaml:"search"`
	Pools       CreditPools         `yaml:"-"`
	Providers   map[string]Provider `yaml:"-"`
	Tenants     map[string]Tenant   `yaml:"-"`
}

// Search configures knowledge search (design section 13). The zero value is full text only.
type Search struct {
	// Embedder names the provider that makes embeddings. Empty means none: no vectors are
	// made and nothing leaves the machine for search.
	Embedder string `yaml:"embedder"`
}

// Provider is one line of providers.yaml.
type Provider struct {
	Harness     string `yaml:"harness"`
	Auth        string `yaml:"auth"`
	Concurrency int    `yaml:"concurrency"`
	Pool        string `yaml:"pool"`
	Model       string `yaml:"model"`
	BaseURL     string `yaml:"base_url"`
	KeyEnv      string `yaml:"key_env"`
	// Price turns token counts into dollars for a harness that reports no cost (codex).
	Price Price `yaml:"price"`
	// Tier is "primary" (the default) or "fallback": see schemas/providers.json.
	Tier string `yaml:"tier"`
}

// CreditPool holds the instance-wide monthly spending ceiling. A nil ceiling is unrestricted.
type CreditPool struct {
	MonthlyUSD *float64 `yaml:"monthly_usd"`
}

// CreditPools maps names from providers.yaml to their shared spending limits.
type CreditPools map[string]CreditPool

// NeedsCreditReservation reports whether a job must reserve its maximum cost before it runs.
func (pools CreditPools) NeedsCreditReservation(p Provider) bool {
	return p.Pool != "" && pools[p.Pool].MonthlyUSD != nil
}

// Price is dollars per million tokens. The zero value means "unknown": the cost stays what the
// harness reported.
type Price struct {
	InputPerMTok  float64 `yaml:"input_per_mtok"`
	OutputPerMTok float64 `yaml:"output_per_mtok"`
}

// Tenant is a tenant.yaml.
type Tenant struct {
	Name       string   `yaml:"name"`
	Governance string   `yaml:"governance"`
	Repos      []string `yaml:"repos"`
	People     People   `yaml:"people"`
	Chat       Adapter  `yaml:"chat"`
	Tracker    Adapter  `yaml:"tracker"`
	MCP        struct {
		Remote struct {
			Listen string `yaml:"listen"`
		} `yaml:"remote"`
	} `yaml:"mcp"`
	Providers map[string][]string `yaml:"providers"`
	OnLimit   struct {
		MaxWait  string `yaml:"max_wait"`
		Fallback string `yaml:"fallback"`
	} `yaml:"on_limit"`
	Review   Review   `yaml:"review"`
	Schedule Schedule `yaml:"schedule"`
	Budget   struct {
		// Weight is the tenant's share of the workers against other tenants; zero means 1.
		Weight float64    `yaml:"weight"`
		Task   TaskBudget `yaml:"task"`
		// Run overrides the limits of a role for one agent run of this tenant.
		Run RoleLimits `yaml:"run"`
	} `yaml:"budget"`
	// Roles overrides the model and effort of a role for this tenant (design section 12).
	Roles map[string]RoleRun `yaml:"roles"`
	// Capacity bounds what the tenant runs at once, so one tenant cannot take the whole
	// instance (design section 17). Zero means no limit.
	Capacity struct {
		Tasks int `yaml:"tasks"`
		// Previews bounds the previews that run at once; a task waits in preview for a free one.
		// Unset means no limit; 0 means the factory starts no previews for this tenant.
		Previews *int `yaml:"previews"`
	} `yaml:"capacity"`
	// Language is the tenant's chat language: "auto" (empty means the same) follows the owner's
	// message; a two-letter code fixes it. Artifacts are always English.
	Language struct {
		Artifacts string `yaml:"artifacts"`
		Chat      string `yaml:"chat"`
	} `yaml:"language"`
}

// Review is the tenant's review switch (design section 11); the policy package reads it.
type Review struct {
	Mode    string            `yaml:"mode"`
	Actions map[string]string `yaml:"actions"`
	Sample  float64           `yaml:"sample"`
	// RemindAfterHours lists how long a review may wait before the owner is reminded: [4, 24].
	RemindAfterHours []int `yaml:"remind_after_hours"`
	// SLAAutomergeHours approves a low blast-radius pull request that no person reviewed in this
	// many hours; zero (the default) switches it off. The schema forbids it for the team profile.
	SLAAutomergeHours int `yaml:"sla_automerge_hours"`
}

// People names who may decide: commands are accepted only from them (design section 7).
type People struct {
	Owner struct {
		Chat   string `yaml:"chat"`
		GitHub string `yaml:"github"`
	} `yaml:"owner"`
	Approvers []string `yaml:"approvers"`
}

// Adapter selects an integration and its settings.
type Adapter struct {
	Adapter  string            `yaml:"adapter"`
	Channels map[string]string `yaml:"channels"`
}

// TaskBudget is the default budget of one task.
type TaskBudget struct {
	Iterations int   `yaml:"iterations"`
	Tokens     int64 `yaml:"tokens"`
	Hours      int   `yaml:"hours"`
}

type instanceFile struct {
	Instance  `yaml:",inline"`
	Providers string `yaml:"providers"`
	Tenants   string `yaml:"tenants"`
}

// LoadInstance reads dir/instance.yaml, its providers file and every tenant pack
// (tenants/<name>/tenant.yaml), validates each against its schema and checks references
// between them. Any error stops the load: the service does not start half-configured.
func LoadInstance(v *Validator, dir string) (*Instance, error) {
	var f instanceFile
	if err := loadFile(v, KindInstance, filepath.Join(dir, "instance.yaml"), &f); err != nil {
		return nil, err
	}
	inst := f.Instance
	providers := map[string]yaml.Node{}
	if err := loadFile(v, KindProviders, within(dir, f.Providers), &providers); err != nil {
		return nil, err
	}
	inst.Providers = map[string]Provider{}
	for name, node := range providers {
		if name == "pools" {
			if err := node.Decode(&inst.Pools); err != nil {
				return nil, fmt.Errorf("provider pools: %w", err)
			}
			continue
		}
		var p Provider
		if err := node.Decode(&p); err != nil {
			return nil, fmt.Errorf("provider %s: %w", name, err)
		}
		inst.Providers[name] = p
	}
	tenants, err := loadTenants(v, within(dir, f.Tenants))
	if err != nil {
		return nil, err
	}
	inst.Tenants = tenants
	if err := inst.check(); err != nil {
		return nil, err
	}
	return &inst, nil
}

func loadTenants(v *Validator, dir string) (map[string]Tenant, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read tenants: %w", err)
	}
	out := map[string]Tenant{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var t Tenant
		if err := loadFile(v, KindTenant, filepath.Join(dir, e.Name(), "tenant.yaml"), &t); err != nil {
			return nil, err
		}
		if t.Name != e.Name() {
			return nil, fmt.Errorf("tenant %s: name %q must match its directory", e.Name(), t.Name)
		}
		out[t.Name] = t
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no tenant packs in %s", dir)
	}
	return out, nil
}

// check finds references the schemas cannot see: a tenant may use only its instance's
// providers, and a repository belongs to one tenant. Tenants run in parallel loops and share
// the repository mirrors, so two tenants on one repository would prune each other's task
// branches (design section 17).
func (inst *Instance) check() error {
	var errs []error
	names := make([]string, 0, len(inst.Tenants))
	for name := range inst.Tenants {
		names = append(names, name)
	}
	sort.Strings(names)
	errs = append(errs, inst.sharedRepos(names)...)
	for _, name := range slices.Sorted(maps.Keys(inst.Providers)) {
		p := inst.Providers[name]
		if _, ok := inst.Pools[p.Pool]; p.Pool != "" && !ok {
			errs = append(errs, fmt.Errorf("provider %s names undefined credit pool %q", name, p.Pool))
		}
	}
	for _, name := range names {
		t := inst.Tenants[name]
		roles := make([]string, 0, len(t.Providers))
		for role := range t.Providers {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		for _, role := range roles {
			for _, p := range t.Providers[role] {
				if _, ok := inst.Providers[p]; !ok {
					errs = append(errs, fmt.Errorf("tenant %s: role %s uses provider %q, which instance %s does not have", name, role, p, inst.Name))
				}
			}
		}
	}
	return errors.Join(errs...)
}

// sharedRepos reports every repository that a second tenant names. Mirrors are keyed by
// owner/name, so neither the host nor the case tells two apart.
func (inst *Instance) sharedRepos(names []string) []error {
	var errs []error
	owner := map[string]string{}
	for _, name := range names {
		for _, repo := range inst.Tenants[name].Repos {
			key := strings.ToLower(repo)
			if parts := strings.Split(key, "/"); len(parts) == 3 {
				key = parts[1] + "/" + parts[2]
			}
			if other, ok := owner[key]; ok {
				errs = append(errs, fmt.Errorf("tenant %s: repository %s already belongs to tenant %s", name, repo, other))
				continue
			}
			owner[key] = name
		}
	}
	return errs
}

func loadFile(v *Validator, kind Kind, path string, out any) error {
	raw, err := os.ReadFile(path) //nolint:gosec // paths come from the operator's config directory
	if err != nil {
		return fmt.Errorf("read %s: %w", kind, err)
	}
	if err := v.ValidateYAML(kind, raw); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: decode: %w", path, err)
	}
	return nil
}

// within resolves a path from instance.yaml relative to the instance directory.
func within(dir, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(dir, rel)
}
