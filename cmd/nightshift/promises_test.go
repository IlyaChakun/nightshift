package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The tests in this file check promises against the code, in the style of design_test.go: what a
// prompt, a schema or a role file says the factory does must be something the code does.
// Something planned but not built goes into an allowlist below with the id of an open item in docs/plan.md,
// so every gap has an owner item; an entry whose gap is closed fails the test and must go.

// promiseGap is an accepted gap between a promise and the code.
type promiseGap struct {
	Plan string // id of an open item (`- [ ] Q1.`) in docs/plan.md that closes the gap
	Why  string
}

// checkGaps fails for an allowlist entry that is not in found (the gap is closed), or whose plan
// phrase is missing from docs/plan.md, and for every found gap that has no entry.
func checkGaps(t *testing.T, label string, found []string, allow map[string]promiseGap) {
	t.Helper()
	plan := readRepoFile(t, "../../docs/plan.md")
	openItem := func(id string) bool {
		return regexp.MustCompile(`(?m)^- \[ \] ` + regexp.QuoteMeta(id) + `\.`).MatchString(plan)
	}
	for _, k := range found {
		if _, ok := allow[k]; !ok {
			t.Errorf("%s: %s", label, k)
		}
	}
	for k, g := range allow {
		if !slices.Contains(found, k) {
			t.Errorf("%s: allowlist entry %q is not a gap any more; remove it", label, k)
		}
		if g.Why == "" || g.Plan == "" || !openItem(g.Plan) {
			t.Errorf("%s: allowlist entry %q needs a reason and an open plan item (a line '- [ ] %s.' in docs/plan.md)", label, k, g.Plan)
		}
	}
}

// goSources parses the non-test Go files of internal/ and cmd/, without generated code.
func goSources(t *testing.T) map[string]*ast.File {
	t.Helper()
	out := map[string]*ast.File{}
	fset := token.NewFileSet()
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(path)] = f
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// promptFiles are the texts an agent reads as instructions.
func promptFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, root := range []string{"../../roles", "../../skills/own", "../../.agents/skills"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".md") {
				out = append(out, path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// promptCommandAllow lists commands that prompts name before they exist. The prompt must say so.
var promptCommandAllow = map[string]promiseGap{}

var promptCommand = regexp.MustCompile("`(nightshift|ns) ([a-z][a-z-]*)")

// TestPromptsNameOnlyExistingCommands checks every `nightshift <command>` and `ns <command>` in
// role prompts and skills: the first must be a subcommand of the binary, the second has no
// command at all yet, so it is allowed only with a plan item.
func TestPromptsNameOnlyExistingCommands(t *testing.T) {
	t.Parallel()
	known := map[string]bool{}
	for _, c := range commands {
		known[c.name] = true
	}
	found := map[string]bool{}
	for _, path := range promptFiles(t) {
		for _, m := range promptCommand.FindAllStringSubmatch(readRepoFile(t, path), -1) {
			if m[1] == "nightshift" && known[m[2]] {
				continue
			}
			found[m[1]+" "+m[2]] = true
		}
	}
	checkGaps(t, "a prompt names a command the binary does not have", slices.Sorted(mapKeys(found)), promptCommandAllow)
}

func mapKeys[V any](m map[string]V) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// vendoredSkillAllow lists skills that roles name from outside sources and that skills/vendor.yaml
// does not vendor yet; until then the worker skips them with a warning.
var vendoredSkillAllow = func() map[string]promiseGap {
	m := map[string]promiseGap{}
	for _, s := range []string{"playwright/playwright-cli"} {
		m[s] = promiseGap{Plan: "Q2", Why: "its text installs a package at runtime, which design section 12 rules out; Verifier acceptance is phase 6"}
	}
	return m
}()

// TestRoleSkillsExist checks that a role lists only skills that are in the repository.
func TestRoleSkillsExist(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("../../roles/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no roles: %v", err)
	}
	var found []string
	for _, f := range files {
		var r struct {
			Skills []string `yaml:"skills"`
		}
		if err := yaml.Unmarshal([]byte(readRepoFile(t, f)), &r); err != nil {
			t.Fatal(err)
		}
		for _, s := range r.Skills {
			dir := filepath.Join("../../skills/vendor", s) // vendored sets
			if strings.HasPrefix(s, "own/") {
				dir = filepath.Join("../../skills", s)
			}
			if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil && !slices.Contains(found, s) {
				found = append(found, s)
			}
		}
	}
	slices.Sort(found)
	checkGaps(t, "a role lists a skill that is not in skills/", found, vendoredSkillAllow)
}

// roleKeyAllow lists role.yaml keys that the schema accepts and the code ignores.
var roleKeyAllow = map[string]promiseGap{}

// TestRoleKeysAreRead checks that every key schemas/role.json allows is read by the code that
// runs roles. A key is read when some non-test code outside the loader selects the matching
// field on a variable whose name contains "role" (`role.Limits.MaxReturns`, `role.ReadOnly()`).
func TestRoleKeysAreRead(t *testing.T) {
	t.Parallel()
	// yaml key -> Go selector names that read it. `name` is read by the loader, which checks it
	// against the file name, so it is not listed.
	selectors := map[string][]string{
		"access": {"Access", "ReadOnly"}, "skills": {"Skills"}, "output_schema": {"OutputSchema"},
		"limits": {"Limits"}, "max_returns": {"MaxReturns"}, "minutes": {"Minutes"}, "tokens": {"Tokens"},
		"stall_minutes": {"StallMinutes"}, "repeat_limit": {"RepeatLimit"}, "cost_usd": {"CostUSD"},
	}
	read := roleSelectorsRead(t)
	var found []string
	for _, key := range schemaKeys(t, "role") {
		if key == "name" {
			continue
		}
		names, ok := selectors[key]
		if !ok {
			t.Errorf("schemas/role.json has key %q; add it to this test's selector table", key)
			continue
		}
		if !slices.ContainsFunc(names, func(n string) bool { return read[n] }) {
			found = append(found, key)
		}
	}
	checkGaps(t, "role.yaml key that no code reads", found, roleKeyAllow)
}

// roleSelectorsRead returns the names selected on a variable whose name contains "role" in
// non-test code outside the role loader.
func roleSelectorsRead(t *testing.T) map[string]bool {
	t.Helper()
	read := map[string]bool{}
	for path, f := range goSources(t) {
		if strings.HasSuffix(path, "internal/config/role.go") {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			chain := selectorChain(sel)
			if len(chain) > 1 && strings.Contains(strings.ToLower(chain[0]), "role") {
				for _, name := range chain[1:] {
					read[name] = true
				}
			}
			return true
		})
	}
	return read
}

// selectorChain returns [a b c] for a.b.c, and nil when the chain does not start with a name.
func selectorChain(e ast.Expr) []string {
	switch e := e.(type) {
	case *ast.Ident:
		return []string{e.Name}
	case *ast.SelectorExpr:
		base := selectorChain(e.X)
		if base == nil {
			return nil
		}
		return append(base, e.Sel.Name)
	case *ast.CallExpr:
		return selectorChain(e.Fun)
	}
	return nil
}

// schemaKeys lists the property names of a schema in schemas/, at any depth. The names under a
// patternProperties or additionalProperties schema (a map key chosen by the user) are not listed.
func schemaKeys(t *testing.T, name string) []string {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(readRepoFile(t, "../../schemas/"+name+".json")), &doc); err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	var walk func(n any)
	walk = func(n any) {
		switch n := n.(type) {
		case map[string]any:
			if props, ok := n["properties"].(map[string]any); ok {
				for k := range props {
					set[k] = true
				}
			}
			for _, v := range n {
				walk(v)
			}
		case []any:
			for _, v := range n {
				walk(v)
			}
		}
	}
	walk(doc)
	return slices.Sorted(mapKeys(set))
}

// mentionedStrings returns every yaml or json struct tag name and every string literal in
// non-test Go code.
func mentionedStrings(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, f := range goSources(t) {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Field:
				addTagNames(out, n.Tag)
			case *ast.BasicLit:
				if s, err := strconv.Unquote(n.Value); err == nil && n.Kind == token.STRING {
					out[s] = true
				}
			}
			return true
		})
	}
	return out
}

func addTagNames(out map[string]bool, lit *ast.BasicLit) {
	if lit == nil {
		return
	}
	tag, err := strconv.Unquote(lit.Value)
	if err != nil {
		return
	}
	for _, k := range []string{"yaml", "json"} {
		name, _, _ := strings.Cut(reflect.StructTag(tag).Get(k), ",")
		out[name] = true
	}
}

// configKeyAllow lists schema fields that are validated but that no code reads yet. Each one is a
// promise to a person writing a pack: the factory must read it or the schema must lose it.
var configKeyAllow = func() map[string]promiseGap {
	m := map[string]promiseGap{}
	add := func(why string, keys ...string) {
		for _, k := range keys {
			m[k] = promiseGap{Plan: "Q1", Why: why}
		}
	}
	add("phase 6 (P1) builds the Deployer that reads capabilities",
		"project.capabilities", "project.merge_train", "project.external", "project.envs")
	add("phase 6 (P4): the compose-local adapter reads the recipe limits and the entry service",
		"project.entry", "project.cpus")
	add("review reminders are an open item of phase 3 item 10", "tenant.remind_after_hours")
	add("design promises it; no code reads it yet",
		"tenant.previews", "tenant.resources", "tenant.cpus",
		"tenant.language", "tenant.quiet_hours", "tenant.releases_in_quiet_hours", "tenant.sla_automerge_hours",
		"instance.allow_ci", "instance.registry_mirrors", "providers.caps", "providers.monthly_usd")
	return m
}()

// TestSchemaFieldsAreRead checks that every property of the project, tenant, instance and
// providers schemas appears in non-test Go code as a yaml or json tag or as a string. It checks
// the key name, not its path, so a common word that is used elsewhere hides a gap; it still finds
// the fields that no code mentions at all.
func TestSchemaFieldsAreRead(t *testing.T) {
	t.Parallel()
	mentioned := mentionedStrings(t)
	var found []string
	for _, schema := range []string{"project", "tenant", "instance", "providers"} {
		for _, key := range schemaKeys(t, schema) {
			if !mentioned[key] {
				found = append(found, schema+"."+key)
			}
		}
	}
	checkGaps(t, "schema field that no code reads", found, configKeyAllow)
}
