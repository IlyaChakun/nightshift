package policy

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
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
	b.WriteString("(?i)^")
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
