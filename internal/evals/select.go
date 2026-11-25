// Package evals decides which evals a change triggers (design section 20). It reads a rules
// file and a list of changed paths; running the evals is another step.
package evals

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Rule maps change paths to evals.
type Rule struct {
	Name         string   `yaml:"name"`
	Paths        []string `yaml:"paths"`
	Roles        []string `yaml:"roles"`
	RoleFromPath bool     `yaml:"role_from_path"`
	Levels       []string `yaml:"levels"`
	Sets         []string `yaml:"sets"`
}

// Run is one eval the change triggers.
type Run struct {
	Set, Level, Role string
}

// Parse reads the rules and rejects one that cannot trigger anything.
func Parse(doc []byte) ([]Rule, error) {
	var rules []Rule
	if err := yaml.Unmarshal(doc, &rules); err != nil {
		return nil, fmt.Errorf("evals: %w", err)
	}
	for i, r := range rules {
		if len(r.Paths) == 0 || len(r.Levels) == 0 || len(r.Sets) == 0 {
			return nil, fmt.Errorf("evals: rule %d (%q) needs paths, levels and sets", i+1, r.Name)
		}
		for _, l := range r.Levels {
			if l != "E1" && l != "E2" && l != "E3" {
				return nil, fmt.Errorf("evals: rule %q: unknown level %q", r.Name, l)
			}
		}
		for _, s := range r.Sets {
			if !slices.Contains([]string{"smoke", "fast", "full", "holdout"}, s) {
				return nil, fmt.Errorf("evals: rule %q: unknown set %q", r.Name, s)
			}
		}
	}
	return rules, nil
}

// Select returns the evals the changed paths trigger, without duplicates and in a stable order.
// An empty Role means the eval is not tied to one role.
func Select(rules []Rule, changed []string) []Run {
	seen := map[Run]bool{}
	for _, p := range changed {
		for _, r := range rules {
			if !matchesAny(r.Paths, p) {
				continue
			}
			roles := r.Roles
			if r.RoleFromPath {
				roles = []string{strings.TrimSuffix(path.Base(p), path.Ext(p))}
			}
			if len(roles) == 0 {
				roles = []string{""}
			}
			for _, role := range roles {
				for _, set := range r.Sets {
					for _, level := range r.Levels {
						seen[Run{Set: set, Level: level, Role: role}] = true
					}
				}
			}
		}
	}
	out := make([]Run, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Set != b.Set {
			return a.Set < b.Set
		}
		if a.Role != b.Role {
			return a.Role < b.Role
		}
		return a.Level < b.Level
	})
	return out
}

func matchesAny(globs []string, p string) bool {
	return slices.ContainsFunc(globs, func(g string) bool { return match(g, p) })
}

// match is a glob where `*` stays inside one path segment and `**` crosses segments.
func match(glob, p string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch {
		case strings.HasPrefix(glob[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(glob[i:], "**"):
			b.WriteString(".*")
			i++
		case glob[i] == '*':
			b.WriteString("[^/]*")
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	return err == nil && re.MatchString(p)
}
