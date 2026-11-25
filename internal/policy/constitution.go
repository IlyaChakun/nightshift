package policy

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
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
// (the command scripts from project.yaml and its `protected` list).
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
	var hit []string
	for _, p := range paths {
		if clean := path.Clean(p); clean != p || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "../") || c.protected(p) {
			hit = append(hit, p)
		}
	}
	if len(hit) > 0 {
		return fmt.Errorf("%w: %s", ErrProtected, strings.Join(hit, ", "))
	}
	return nil
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
			var p struct {
				Protected []string `yaml:"protected"`
			}
			if err := yaml.Unmarshal([]byte(doc), &p); err != nil {
				return fmt.Errorf("policy: project.yaml: %w", err)
			}
			extra = p.Protected
		}
		c, err := NewConstitution(extra)
		if err != nil {
			return err
		}
		return c.Check(paths)
	}
}
