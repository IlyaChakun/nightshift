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
	On           []string   `yaml:"on"`
	FreshContext bool       `yaml:"fresh_context"`
	Access       string     `yaml:"access"` // read_only or worktree
	Skills       []string   `yaml:"skills"`
	Inputs       []string   `yaml:"inputs"`
	OutputSchema string     `yaml:"output_schema"`
	Limits       RoleLimits `yaml:"limits"`
}

// RoleLimits bound one run of a role.
type RoleLimits struct {
	MaxReturns int `yaml:"max_returns"`
	Minutes    int `yaml:"minutes"`
	Tokens     int `yaml:"tokens"`
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
	if _, err := fs.Stat(schemas.FS, path.Base(r.OutputSchema)); err != nil {
		return Role{}, fmt.Errorf("output_schema %s does not exist", r.OutputSchema)
	}
	for _, in := range r.Inputs {
		if !knownInputs[in] {
			return Role{}, fmt.Errorf("input %q is not one nightshift hands to a role", in)
		}
	}
	return r, nil
}

// knownInputs are what a job carries: the thread (Lead's prompt), the standards (the L0
// checklist), the spec, open findings, the diff and the hidden scenarios (Verifier only). A
// name outside the list would be ignored, so the file is refused instead.
var knownInputs = map[string]bool{
	"thread": true, "standards": true, "spec": true, "findings": true, "diff_path": true, "hidden_scenarios": true,
}
