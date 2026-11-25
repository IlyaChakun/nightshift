package onboard

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/stack"
)

// Status of one check.
type Status string

// Statuses: a failed check blocks the connection, a warning only informs the owner's choice
// of review mode (design section 17, step 7).
const (
	OK   Status = "ok"
	Warn Status = "warn"
	Fail Status = "fail"
)

// Result is one line of the doctor's report.
type Result struct {
	Name   string
	Status Status
	Detail string
	Fix    string
}

// Doctor checks that a tenant can be connected: the project file, the tools the worker
// needs, the secrets, and how ready the repository is for agents.
type Doctor struct {
	Tenant config.Tenant
	Repo   fs.FS
	// Secret reads one secret of the instance; an error means it is missing or unreadable.
	Secret func(name string) error
	// Look finds an executable on the worker's PATH.
	Look      func(name string) error
	Validator *config.Validator
	// Packs are the stack packs; without them the doctor knows only a built-in list.
	Packs stack.Packs
}

// toolchains is what the worker needs to run a stack's verify command.
var toolchains = map[string][]string{"go": {"go"}, "python": {"python3"}, "ts": {"node"}, "js": {"node"}}

// Run returns the report. Failures come first in the order of the steps; the readiness
// warnings follow.
func (d Doctor) Run() []Result {
	var out []Result
	project, ok := d.project(&out)
	out = append(out, d.tools(project, ok)...)
	out = append(out, d.secrets()...)
	return append(out, d.readiness(project)...)
}

// Failed reports whether any check failed.
func Failed(rs []Result) bool {
	return slices.ContainsFunc(rs, func(r Result) bool { return r.Status == Fail })
}

type projectFile struct {
	Stack  []string `yaml:"stack"`
	Verify string   `yaml:"verify"`
}

func (d Doctor) project(out *[]Result) (projectFile, bool) {
	var p projectFile
	doc, err := fs.ReadFile(d.Repo, ".nightshift/project.yaml")
	if err != nil {
		*out = append(*out, Result{"project.yaml", Fail, "`.nightshift/project.yaml` is missing", "run `nightshift tenant init <name> --discover` and the onboard-tenant skill, then merge the PR"})
		return p, false
	}
	if err := d.Validator.ValidateYAML(config.KindProject, doc); err != nil {
		*out = append(*out, Result{"project.yaml", Fail, err.Error(), "fix the file against schemas/project.json"})
		return p, false
	}
	if err := yaml.Unmarshal(doc, &p); err != nil {
		*out = append(*out, Result{"project.yaml", Fail, err.Error(), "fix the YAML"})
		return p, false
	}
	*out = append(*out, Result{Name: "project.yaml", Status: OK, Detail: "valid"})
	return p, true
}

func (d Doctor) tools(p projectFile, ok bool) []Result {
	if !ok {
		return nil
	}
	var out []Result
	var need []string
	for _, s := range p.Stack {
		if pack, ok := d.Packs[s]; ok {
			need = append(need, pack.Toolchain...)
		} else {
			need = append(need, toolchains[s]...)
		}
	}
	if first := strings.Fields(p.Verify); len(first) > 0 {
		need = append(need, first[0])
	}
	slices.Sort(need)
	for _, tool := range slices.Compact(need) {
		if err := d.Look(tool); err != nil {
			out = append(out, Result{"tool " + tool, Fail, "the worker image cannot run `" + tool + "`", "add the toolchain to the worker image (stack pack) and rebuild"})
		} else {
			out = append(out, Result{Name: "tool " + tool, Status: OK, Detail: "found"})
		}
	}
	return out
}

func (d Doctor) secrets() []Result {
	need := []string{"github_token"}
	if a := d.Tenant.Chat.Adapter; a != "" && a != "github-only" {
		need = append(need, "chat_bot_token", "chat_app_token")
	}
	var out []Result
	for _, name := range need {
		if err := d.Secret(name); err != nil {
			out = append(out, Result{"secret " + name, Fail, err.Error(), "put the value into the secrets directory by hand (docs/secrets.md)"})
		} else {
			out = append(out, Result{Name: "secret " + name, Status: OK, Detail: "readable"})
		}
	}
	return out
}

// readiness is the agent readiness report: what exists that agents can lean on, and what
// to add. It never fails; it informs the choice of review mode.
func (d Doctor) readiness(p projectFile) []Result {
	has := func(names ...string) bool {
		return slices.ContainsFunc(names, func(n string) bool { _, err := fs.Stat(d.Repo, n); return err == nil })
	}
	var configs int
	for _, f := range toolConfigs {
		if has(f) {
			configs++
		}
	}
	items := []struct {
		name    string
		present bool
		fix     string
	}{
		{"build and tests", p.Verify != "", "set `verify` in project.yaml to one command that builds and tests"},
		{"linters and types", configs > 0, "add the stack's linter and type checker configs; `standards sync` can propose them"},
		{"project instructions", has("AGENTS.md", "CLAUDE.md", ".github/copilot-instructions.md"), "add a short AGENTS.md (an index, about 100 lines)"},
		{"reproducible dev environment", has("mise.toml", ".tool-versions", ".devcontainer/devcontainer.json", "flake.nix", "Dockerfile"), "pin tool versions (mise.toml or a devcontainer)"},
		{"CI", has(".github/workflows", ".gitlab-ci.yml"), "add a CI workflow that runs `verify` on pull requests"},
		{"security checks", has(".gitleaks.toml", ".github/dependabot.yml", "SECURITY.md"), "add secret scanning and dependency updates"},
	}
	var out []Result
	for _, it := range items {
		if it.present {
			out = append(out, Result{Name: "ready: " + it.name, Status: OK, Detail: "present"})
		} else {
			out = append(out, Result{"ready: " + it.name, Warn, "missing", it.fix})
		}
	}
	return out
}

// Format renders the report for a terminal.
func Format(rs []Result) string {
	var b strings.Builder
	for _, r := range rs {
		mark := map[Status]string{OK: "ok  ", Warn: "warn", Fail: "FAIL"}[r.Status]
		fmt.Fprintf(&b, "%s  %s", mark, r.Name)
		if r.Detail != "" {
			fmt.Fprintf(&b, ": %s", r.Detail)
		}
		if r.Fix != "" && r.Status != OK {
			fmt.Fprintf(&b, "\n      fix: %s", r.Fix)
		}
		b.WriteString("\n")
	}
	return b.String()
}
