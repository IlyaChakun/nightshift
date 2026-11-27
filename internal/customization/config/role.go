package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/schemas"
)

// Role is roles/<name>.yaml: a role is configuration (design section 6).
type Role struct {
	Name         string     `yaml:"name"`
	Access       string     `yaml:"access"` // read_only or worktree
	Skills       []string   `yaml:"skills"`
	OutputSchema string     `yaml:"output_schema"`
	Limits       RoleLimits `yaml:"limits"`
	// Model is the role's model per harness (claude, codex, ...), a full model id so a CLI
	// update cannot change it. A provider's own model wins: it is all that provider serves.
	Model map[string]string `yaml:"model"`
	// Effort is the reasoning effort of the role: low, medium, high, xhigh or max. Empty keeps
	// the harness default.
	Effort string `yaml:"effort"`
}

// RoleRun is a tenant's choice of model and effort for one role, over the role's own.
type RoleRun struct {
	Model  map[string]string `yaml:"model"`
	Effort string            `yaml:"effort"`
}

// RoleLimits bound one run of a role.
type RoleLimits struct {
	MaxReturns int `yaml:"max_returns"`
	Minutes    int `yaml:"minutes"`
	Tokens     int `yaml:"tokens"`
	// The supervisor's limits (ADR 0019). Absent keeps the worker's default; 0 switches the rule off.
	StallMinutes *int     `yaml:"stall_minutes"`
	RepeatLimit  *int     `yaml:"repeat_limit"`
	CostUSD      *float64 `yaml:"cost_usd"`
}

// ReadOnly reports whether the role may not write to its worktree.
func (r Role) ReadOnly() bool { return r.Access == "read_only" }

// LoadRoles reads every *.yaml in the root of fsys, validates it against schemas/role.json
// and checks what the schema cannot: the file is named after the role, and the output
// schema exists. It returns the roles by name.
func LoadRoles(fsys fs.FS) (map[string]Role, error) {
	files, err := fs.Glob(fsys, "*.yaml")
	if err != nil {
		return nil, fmt.Errorf("config: roles: %w", err)
	}
	v := NewValidator()
	roles := make(map[string]Role, len(files))
	var errs []error
	for _, file := range files {
		r, err := loadRole(v, fsys, file)
		if err != nil {
			errs = append(errs, fmt.Errorf("roles/%s: %w", file, err))
			continue
		}
		roles[r.Name] = r
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return roles, nil
}

func loadRole(v *Validator, fsys fs.FS, file string) (Role, error) {
	doc, err := fs.ReadFile(fsys, file)
	if err != nil {
		return Role{}, err
	}
	if err := v.ValidateYAML(KindRole, doc); err != nil {
		return Role{}, err
	}
	var r Role
	if err := yaml.Unmarshal(doc, &r); err != nil {
		return Role{}, err
	}
	if want := strings.TrimSuffix(file, ".yaml"); r.Name != want {
		return Role{}, fmt.Errorf("name %q must match the file name %q", r.Name, want)
	}
	if _, err := fs.Stat(schemas.FS(), path.Base(r.OutputSchema)); err != nil {
		return Role{}, fmt.Errorf("output_schema %s does not exist", r.OutputSchema)
	}
	return r, nil
}
