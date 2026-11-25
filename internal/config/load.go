package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"
)

// Instance is an instance.yaml with its providers and the tenant packs it serves.
// Only the fields the core reads are typed; the schemas check the whole file.
type Instance struct {
	Name        string              `yaml:"name"`
	TrustDomain string              `yaml:"trust_domain"`
	Search      Search              `yaml:"search"`
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
	Review   Review `yaml:"review"`
	Schedule struct {
		Timezone       string `yaml:"timezone"`
		MorningSummary string `yaml:"morning_summary"` // "09:00"; the morning digest goes out after it
	} `yaml:"schedule"`
	Budget struct {
		// Weight is the tenant's share of the workers against other tenants; zero means 1.
		Weight float64    `yaml:"weight"`
		Task   TaskBudget `yaml:"task"`
	} `yaml:"budget"`
	// Capacity bounds what the tenant runs at once, so one tenant cannot take the whole
	// instance (design section 17). Zero means no limit.
	Capacity struct {
		Tasks int `yaml:"tasks"`
	} `yaml:"capacity"`
}

// Review is the tenant's review switch (design section 11); the policy package reads it.
type Review struct {
	Mode    string            `yaml:"mode"`
	Actions map[string]string `yaml:"actions"`
	Sample  float64           `yaml:"sample"`
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

// check finds references the schemas cannot see: a tenant may use only its instance's providers.
func (inst *Instance) check() error {
	var errs []error
	names := make([]string, 0, len(inst.Tenants))
	for name := range inst.Tenants {
		names = append(names, name)
	}
	sort.Strings(names)
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
