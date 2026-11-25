// Package onboard reads a repository without a model and drafts what Nightshift needs to know
// about it: stacks, instruction files, the verify command and what is left to ask (design
// section 17, step 4). It never edits the repository.
package onboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/stack"
)

// Report is what discovery found.
type Report struct {
	Name         string
	Stacks       []string
	Instructions []string
	Skills       []string
	Verify       string
	// Configs are tool configuration files found; they feed `standards sync`, not project.yaml.
	Configs []string
	// Onboarded is true when the repository already has .nightshift/project.yaml.
	Onboarded bool
	Gaps      []string
	// Unchecked are build files of a language that has no pack: its patches get no static checks.
	Unchecked []string
}

// instructionFiles are the places agents' instructions live, in the order they are listed.
var instructionFiles = []string{"AGENTS.md", "CLAUDE.md", ".github/copilot-instructions.md"}

// toolConfigs are the configuration files that tell what the project already enforces.
var toolConfigs = []string{
	".golangci.yml", ".golangci.yaml", "ruff.toml", ".ruff.toml", "mypy.ini", ".editorconfig",
	".eslintrc.json", "eslint.config.js", "eslint.config.mjs", "biome.json", ".prettierrc", "tsconfig.json",
}

var makeTarget = regexp.MustCompile(`^([A-Za-z0-9_.-]+):`)

// Discover inspects the repository tree. name is the tenant's name, used for project.yaml.
// The stacks come from the packs' detect files; without packs a built-in list of the three
// first stacks is used, so a machine with no stacks directory still finds something.
func Discover(fsys fs.FS, name string, packs stack.Packs) Report {
	r := Report{Name: name}
	if len(packs) > 0 {
		r.Stacks = packs.Detect(fsys)
	} else {
		r.Stacks = stacks(fsys)
	}
	r.Unchecked = uncheckedManifests(fsys, r.Stacks, packs)
	for _, f := range instructionFiles {
		if exists(fsys, f) {
			r.Instructions = append(r.Instructions, f)
		}
	}
	r.Instructions = append(r.Instructions, filesIn(fsys, ".cursor/rules")...)
	for _, dir := range []string{".nightshift/skills", ".claude/skills"} {
		if isDir(fsys, dir) {
			r.Skills = append(r.Skills, dir)
		}
	}
	for _, f := range toolConfigs {
		if exists(fsys, f) {
			r.Configs = append(r.Configs, f)
		}
	}
	r.Verify = verifyCommand(fsys)
	r.Onboarded = exists(fsys, ".nightshift/project.yaml")
	r.Gaps = gaps(r, fsys)
	return r
}

// manifests are build files that say a language is in use, whether or not a pack covers it.
var manifests = []string{"pom.xml", "build.gradle", "build.gradle.kts", "Cargo.toml", "Gemfile", "composer.json", "mix.exs", "Package.swift", "CMakeLists.txt"}

// uncheckedManifests returns the build files in the root that no detected pack accounts for.
func uncheckedManifests(fsys fs.FS, found []string, packs stack.Packs) []string {
	covered := map[string]bool{}
	for _, name := range found {
		for _, f := range packs[name].Detect {
			covered[f] = true
		}
	}
	var out []string
	for _, f := range manifests {
		if exists(fsys, f) && !covered[f] {
			out = append(out, f)
		}
	}
	return out
}

func stacks(fsys fs.FS) []string {
	var out []string
	if exists(fsys, "go.mod") {
		out = append(out, "go")
	}
	if exists(fsys, "pyproject.toml") || exists(fsys, "requirements.txt") || exists(fsys, "setup.py") {
		out = append(out, "python")
	}
	if exists(fsys, "package.json") {
		if exists(fsys, "tsconfig.json") {
			out = append(out, "ts")
		} else {
			out = append(out, "js")
		}
	}
	return out
}

// verifyCommand finds the project's own entry point for "check everything": a `verify` target
// in the Makefile first, then `test`, then the `test` script of package.json. It does not guess.
func verifyCommand(fsys fs.FS) string {
	if f, err := fsys.Open("Makefile"); err == nil {
		defer func() { _ = f.Close() }()
		targets := map[string]bool{}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if m := makeTarget.FindStringSubmatch(sc.Text()); m != nil {
				targets[m[1]] = true
			}
		}
		for _, t := range []string{"verify", "check", "test"} {
			if targets[t] {
				return "make " + t
			}
		}
	}
	if raw, err := fs.ReadFile(fsys, "package.json"); err == nil {
		var p struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(raw, &p) == nil {
			if _, ok := p.Scripts["test"]; ok {
				return "npm test"
			}
		}
	}
	return ""
}

func gaps(r Report, fsys fs.FS) []string {
	var g []string
	if len(r.Stacks) == 0 {
		g = append(g, "Stack: no go.mod, pyproject.toml or package.json found. Which languages does the project use?")
	}
	if len(r.Unchecked) > 0 {
		g = append(g, "Stack: "+listOrNone(r.Unchecked)+" has no stack pack, so patches get no static checks. Which command checks that code (it goes into `verify`), and which toolchain does the worker need?")
	}
	if r.Verify == "" {
		g = append(g, "`verify`: no Makefile target `verify`, `check` or `test` and no npm `test` script. Which one command runs build, tests, linters and types?")
	}
	if len(r.Instructions) == 0 {
		g = append(g, "Instructions: no AGENTS.md, CLAUDE.md or other agent instructions. `tenant init` will propose a short AGENTS.md draft for review.")
	}
	if !isDir(fsys, ".github/workflows") && !exists(fsys, ".gitlab-ci.yml") {
		g = append(g, "CI: no workflow files found. How are tests run on a pull request, and which check names count?")
	}
	return append(g,
		"Profile: `solo` or `team`?",
		"People: who is the owner and who may approve (chat and GitHub names)?",
		"Blast radius: which paths and tags are `high` or `critical`, and which are never to be lowered? (`.nightshift/blast-radius.yaml`)",
		"Preview: none, `script` or `external` environments? Without one, acceptance works from test results and logs.",
		"Test levels: which tests exist (unit, integration, e2e) and which of them need a person or an external system?",
		"Merge train: may `nightshift` merge pull requests in this repository (solo) or only hand them off (team)?",
	)
}

type projectDraft struct {
	Name         string   `yaml:"name"`
	Stack        []string `yaml:"stack,omitempty"`
	Instructions []string `yaml:"instructions,omitempty"`
	Skills       []string `yaml:"skills,omitempty"`
	Verify       string   `yaml:"verify,omitempty"`
}

// ProjectYAML is the draft of .nightshift/project.yaml with what was found. It is not valid
// against the schema yet: capabilities and blast_radius come from the interview.
func (r Report) ProjectYAML() ([]byte, error) {
	out, err := yaml.Marshal(projectDraft{Name: r.Name, Stack: r.Stacks, Instructions: r.Instructions, Skills: r.Skills, Verify: r.Verify})
	if err != nil {
		return nil, fmt.Errorf("onboard: %w", err)
	}
	header := "# Draft from `nightshift tenant init --discover`. Not valid yet: the interview adds\n# capabilities and blast_radius (see gaps.md).\n"
	return append([]byte(header), out...), nil
}

// GapsMD lists what discovery could not infer, for the onboard-tenant skill to ask about.
func (r Report) GapsMD() string {
	var b strings.Builder
	b.WriteString("# Gaps for " + r.Name + "\n\n")
	if r.Onboarded {
		b.WriteString("The repository already has `.nightshift/project.yaml`. Review it instead of starting over.\n\n")
	}
	b.WriteString("## Found\n\n")
	b.WriteString("- Stacks: " + listOrNone(r.Stacks) + "\n")
	b.WriteString("- Agent instructions: " + listOrNone(r.Instructions) + "\n")
	b.WriteString("- Project skills: " + listOrNone(r.Skills) + "\n")
	b.WriteString("- Verify command: " + listOrNone([]string{r.Verify}) + "\n")
	b.WriteString("- Tool configs (input for `standards sync`): " + listOrNone(r.Configs) + "\n\n")
	b.WriteString("## To ask the owner\n\n")
	for _, g := range r.Gaps {
		b.WriteString("- " + g + "\n")
	}
	return b.String()
}

func listOrNone(items []string) string {
	items = slices.DeleteFunc(slices.Clone(items), func(s string) bool { return s == "" })
	if len(items) == 0 {
		return "none"
	}
	return "`" + strings.Join(items, "`, `") + "`"
}

func exists(fsys fs.FS, name string) bool {
	_, err := fs.Stat(fsys, name)
	return err == nil
}

func isDir(fsys fs.FS, name string) bool {
	st, err := fs.Stat(fsys, name)
	return err == nil && st.IsDir()
}

func filesIn(fsys fs.FS, dir string) []string {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, path.Join(dir, e.Name()))
		}
	}
	return out
}
