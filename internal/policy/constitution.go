package policy

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// builtinProtected are the paths no Builder patch may touch in any project (design
// section 10, item 8). They are anchored at the repository root.
var builtinProtected = []string{
	".nightshift/**", ".github/**", ".claude/**", ".mcp.json", "opencode.json", ".opencode/**",
	"Makefile", "mise.toml", "mise.lock", "AGENTS.md", "LEARNINGS.md",
}

// ErrProtected means a patch touches a protected path.
var ErrProtected = errors.New("policy: patch touches protected paths")

// Constitution is the list of protected paths: the built-in ones plus those a project adds
// (its `protected` list and the scripts its commands run).
type Constitution struct {
	patterns []*regexp.Regexp
}

// NewConstitution compiles the built-in patterns and the project's extra ones. A pattern is
// a path from the repository root; `*` matches within one directory, `**` across them.
func NewConstitution(extra []string) (*Constitution, error) {
	c := &Constitution{}
	for _, p := range append(append([]string{}, builtinProtected...), extra...) {
		re, err := compileGlob(p)
		if err != nil {
			return nil, err
		}
		c.patterns = append(c.patterns, re)
	}
	return c, nil
}

// compileGlob matches case-insensitively, so a patch cannot slip past on a file system
// that ignores case (makefile, Agents.md).
func compileGlob(glob string) (*regexp.Regexp, error) {
	glob = strings.TrimPrefix(path.Clean(glob), "./")
	if glob == "." || glob == "" || strings.HasPrefix(glob, "/") || strings.HasPrefix(glob, "../") {
		return nil, fmt.Errorf("policy: protected path %q must be relative to the repository root", glob)
	}
	var b strings.Builder
	// s: a newline in a file name must not carry a path past `**`.
	b.WriteString("(?is)^")
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
		case glob[i] == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// Check returns ErrProtected, naming the paths, when any of them is protected. A path that
// leaves the repository root or is not clean is treated as protected: it is not a path
// a Builder patch has any business with.
func (c *Constitution) Check(paths []string) error {
	if hit := c.Hits(paths); len(hit) > 0 {
		return fmt.Errorf("%w: %s", ErrProtected, strings.Join(hit, ", "))
	}
	return nil
}

// Hits returns the paths that are protected, in the order given.
func (c *Constitution) Hits(paths []string) []string {
	var hit []string
	for _, p := range paths {
		if clean := path.Clean(p); clean != p || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "../") || c.protected(p) {
			hit = append(hit, p)
		}
	}
	return hit
}

func (c *Constitution) protected(p string) bool {
	for _, re := range c.patterns {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

// ProjectGuard returns a guard for gitops that checks the built-in paths and the `protected`
// list of .nightshift/project.yaml. The list is read at the base commit, never from the patch
// (`.nightshift/**` is protected anyway), so a patch cannot lift its own protection. A project
// file that does not parse, or a pattern that is not valid, rejects the patch: nothing
// is accepted on a rule that cannot be read.
func ProjectGuard(show func(ctx context.Context, mirror, sha, name string) (string, bool, error)) func(ctx context.Context, mirror, base string, paths []string) error {
	return func(ctx context.Context, mirror, base string, paths []string) error {
		doc, ok, err := show(ctx, mirror, base, ".nightshift/project.yaml")
		if err != nil {
			return fmt.Errorf("policy: read project.yaml: %w", err)
		}
		var extra []string
		if ok {
			f, err := readProjectFile(doc)
			if err != nil {
				return err
			}
			extra = f.guarded()
		}
		c, err := NewConstitution(extra)
		if err != nil {
			return err
		}
		return c.Check(paths)
	}
}

// projectFile is the part of .nightshift/project.yaml that names files which run with more
// rights than the agent has: the commands of `setup` and `verify`, the commands and compose
// recipes of the preview and deploy capabilities, and the project's own `protected` list.
type projectFile struct {
	Protected    []string `yaml:"protected"`
	Setup        string   `yaml:"setup"`
	Verify       string   `yaml:"verify"`
	Capabilities struct {
		Preview  any `yaml:"preview"`
		Release  any `yaml:"release"`
		Rollback any `yaml:"rollback"`
		Smoke    any `yaml:"smoke"`
	} `yaml:"capabilities"`
}

func readProjectFile(doc string) (projectFile, error) {
	var f projectFile
	if err := yaml.Unmarshal([]byte(doc), &f); err != nil {
		return f, fmt.Errorf("policy: project.yaml: %w", err)
	}
	return f, nil
}

// deploy returns the commands and the compose recipe files of the deploy capabilities. A string
// is a project command, an object has `up` and `down` commands (kind script) or a `file` recipe.
func (f projectFile) deploy() (commands, recipes []string) {
	for _, v := range []any{f.Capabilities.Preview, f.Capabilities.Release, f.Capabilities.Rollback, f.Capabilities.Smoke} {
		switch x := v.(type) {
		case string:
			commands = append(commands, x)
		case map[string]any:
			for _, k := range []string{"up", "down"} {
				if c, ok := x[k].(string); ok {
					commands = append(commands, c)
				}
			}
			if r, ok := x["file"].(string); ok {
				if p, ok := repoPath(r); ok {
					recipes = append(recipes, p)
				}
			}
		}
	}
	return commands, recipes
}

// guarded are the paths a Builder patch may not touch: the `protected` list and the scripts
// that every project command runs.
func (f projectFile) guarded() []string {
	commands, _ := f.deploy()
	out := append([]string{}, f.Protected...)
	return append(out, commandPaths(append([]string{f.Setup, f.Verify}, commands...)...)...)
}

// ProjectExecuted returns the repository paths that a privileged service executes or reads as
// its recipe, from the project file at the base commit: everything the Builder guard protects
// plus the compose recipes. A change to one of them is blast radius `critical` whatever the
// project's own rules say, so a rule such as "every shell script is low" cannot hide a script
// that the deployer or the toolchain runs. An unreadable file is an error: the caller holds the
// change for a person.
func ProjectExecuted(doc string) ([]string, error) {
	f, err := readProjectFile(doc)
	if err != nil {
		return nil, err
	}
	_, recipes := f.deploy()
	return append(f.guarded(), recipes...), nil
}

// wrapperWords start a command without being the program it runs.
var wrapperWords = map[string]bool{"env": true, "time": true, "sh": true, "bash": true, "exec": true, "nohup": true}

var scriptExts = map[string]bool{".sh": true, ".bash": true, ".py": true, ".js": true, ".mjs": true, ".ts": true, ".rb": true, ".pl": true, ".ps1": true}

// commandPaths returns the repository files that project commands run, so the agent cannot
// edit the script that checks its own work (design section 10, item 8). `setup`, `verify` and the commands of the
// deploy capabilities are scanned; `tests.ci` names a CI system and `tests.external` is an object. A word is such a path
// when it is relative, stays inside the repository and is either the program a command starts
// with and has a directory part (`./scripts/verify`), or has a script extension anywhere in
// the command (`bash tools/check.sh`). Arguments such as `tests/unit` are not scripts, so
// `pytest tests/unit` protects nothing: the Builder must be able to add tests. Quoted or
// computed paths are not found; the project lists those in `protected`.
func commandPaths(commands ...string) []string {
	var out []string
	for _, c := range commands {
		for _, seg := range strings.FieldsFunc(c, func(r rune) bool { return strings.ContainsRune(";&|()\n", r) }) {
			out = append(out, segmentScripts(seg)...)
		}
	}
	return out
}

// segmentScripts finds the script paths of one command, the part between `;`, `&&` or `|`.
func segmentScripts(seg string) []string {
	var out []string
	first := true // the program of the segment, after wrappers and VAR=x assignments
	for w := range strings.FieldsSeq(seg) {
		if wrapperWords[w] || strings.HasPrefix(w, "-") || (strings.Contains(w, "=") && !strings.Contains(w, "/")) {
			continue
		}
		isProgram := first
		first = false
		if p, ok := repoPath(w); ok && (scriptExts[strings.ToLower(path.Ext(p))] || (isProgram && strings.Contains(p, "/"))) {
			out = append(out, p)
		}
	}
	return out
}

// repoPath returns a word as a path inside the repository, or false for an absolute path, a
// path that leaves the repository, a URL, or a word with shell syntax in it.
func repoPath(w string) (string, bool) {
	w = strings.TrimPrefix(w, "./")
	if w == "" || strings.HasPrefix(w, "/") || strings.HasPrefix(w, "~") || slices.Contains(strings.Split(w, "/"), "..") ||
		strings.Contains(w, "://") || strings.ContainsAny(w, "$`'\"*?[]{}=\\<>") {
		return "", false
	}
	return w, true
}
