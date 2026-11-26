package main

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// Rules that keep the nested AGENTS.md files healthy (root AGENTS.md, "Repository layout"):
// small, loadable by Claude Code, true, present where a directory is big, and never repeating
// the root. Each rule has a planted failing case in TestAgentsLintRules.
const (
	agentsRootWords   = 1800 // the root file loads in every session
	agentsNestedWords = 300  // a nested file loads when a file below it is read
	agentsBigDirFiles = 25   // a top-level directory with more files needs its own AGENTS.md
	agentsShortWords  = 8    // shorter sentences may repeat
)

var (
	agentsSpan      = regexp.MustCompile("`([^`\n]+)`")
	agentsMake      = regexp.MustCompile(`^make( [^ ]+)+$`)
	agentsTarget    = regexp.MustCompile(`(?m)^([A-Za-z0-9_.-]+):`)
	agentsFileExt   = regexp.MustCompile(`\.(md|go|yaml|yml|sh|toml|json|lock|html|py)$`)
	agentsNotAPath  = strings.NewReplacer("<", "\x00", ">", "\x00", "*", "\x00", "{", "\x00", "$", "\x00", "|", "\x00", "=", "\x00", "@", "\x00")
	agentsSplitSent = regexp.MustCompile(`[.;:]\s+|\n`)
)

// skippedDir is data or third-party text the rules do not govern.
func skippedDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "testdata" || name == "evals"
}

// lintAgents returns one problem per violation of the rules above in the tree fsys.
func lintAgents(fsys fs.FS) []string {
	var problems []string
	files := map[string]int{}  // top-level directory -> file count
	names := map[string]bool{} // base names of all files, dot directories included
	var agents []string        // paths of AGENTS.md files
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			names[d.Name()] = true
		}
		if d.IsDir() {
			if p != "." && skippedDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if top, _, ok := strings.Cut(p, "/"); ok {
			files[top]++
		}
		if d.Name() == "AGENTS.md" {
			agents = append(agents, p)
		}
		return nil
	})
	has := map[string]bool{}
	for _, a := range agents {
		has[path.Dir(a)] = true
	}
	for top, n := range files {
		if n > agentsBigDirFiles && !has[top] {
			problems = append(problems, top+"/ has "+strconv.Itoa(n)+" files and no AGENTS.md; add one with a map, entry points, local rules and how to test")
		}
	}
	makeFile, _ := fs.ReadFile(fsys, "Makefile")
	targets := map[string]bool{}
	for _, m := range agentsTarget.FindAllStringSubmatch(string(makeFile), -1) {
		targets[m[1]] = true
	}
	var rootSentences map[string]string
	read := func(p string) string {
		raw, _ := fs.ReadFile(fsys, p)
		return string(raw)
	}
	if text := read("AGENTS.md"); text != "" {
		rootSentences = sentences(text)
	}
	for _, a := range agents {
		dir, text := path.Dir(a), read(a)
		limit := agentsNestedWords
		if dir == "." {
			limit = agentsRootWords
		}
		if n := len(strings.Fields(text)); n > limit {
			problems = append(problems, a+" has "+strconv.Itoa(n)+" words, the limit is "+strconv.Itoa(limit)+"; cut, do not compress")
		}
		if dir != "." && strings.TrimSpace(read(path.Join(dir, "CLAUDE.md"))) != "@AGENTS.md" {
			problems = append(problems, path.Join(dir, "CLAUDE.md")+" must hold the single line @AGENTS.md: Claude Code loads a nested AGENTS.md only through it")
		}
		for _, m := range agentsSpan.FindAllStringSubmatch(text, -1) {
			problems = append(problems, checkSpan(fsys, targets, names, a, dir, m[1])...)
		}
		if dir != "." {
			for s, orig := range sentences(text) {
				if _, dup := rootSentences[s]; dup {
					problems = append(problems, a+" repeats a sentence of the root AGENTS.md; keep it in one place: "+orig)
				}
			}
		}
	}
	return problems
}

// checkSpan checks one backticked text of an AGENTS.md: a path must exist and every `make`
// target must be in the Makefile. Anything else (commands, placeholders, globs, code) is not checked.
func checkSpan(fsys fs.FS, targets, names map[string]bool, file, dir, span string) []string {
	if agentsMake.MatchString(span) {
		var out []string
		for _, t := range strings.Fields(span)[1:] {
			if !strings.HasPrefix(t, "-") && !strings.Contains(t, "=") && !targets[t] {
				out = append(out, file+": `"+span+"` names the make target "+t+", which the Makefile lacks")
			}
		}
		return out
	}
	if strings.ContainsAny(span, " \t") || agentsNotAPath.Replace(span) != span || strings.HasPrefix(span, "http") ||
		strings.HasPrefix(span, "-") || strings.HasPrefix(span, "/") || strings.HasPrefix(span, "~") {
		return nil
	}
	p := path.Clean(strings.TrimPrefix(span, "./"))
	first, _, nested := strings.Cut(p, "/")
	if !nested && !agentsFileExt.MatchString(p) {
		return nil // a word such as a type or a key, not a path
	}
	if !nested && names[p] {
		return nil // a bare file name: some file of that name exists
	}
	for _, base := range []string{".", dir} {
		if _, err := fs.Stat(fsys, path.Join(base, p)); err == nil {
			return nil
		}
	}
	if nested && !agentsFileExt.MatchString(p) {
		// Only a first segment that is a real directory marks a repository path: `log/slog` is not one.
		known := false
		for _, base := range []string{".", dir} {
			if st, err := fs.Stat(fsys, path.Join(base, first)); err == nil && st.IsDir() {
				known = true
			}
		}
		if !known {
			return nil
		}
	}
	return []string{file + ": `" + span + "` does not exist"}
}

// sentences maps the normalized form of each long sentence or line of text to its first words.
func sentences(text string) map[string]string {
	out := map[string]string{}
	for _, s := range agentsSplitSent.Split(text, -1) {
		norm := strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer("`", "", "*", "", "|", " ", "- ", "").Replace(s)), " "))
		if len(strings.Fields(norm)) >= agentsShortWords {
			out[norm] = strings.Join(strings.Fields(norm)[:agentsShortWords], " ") + "..."
		}
	}
	return out
}

// TestAgentsFilesAreHealthy runs the rules on this repository.
func TestAgentsFilesAreHealthy(t *testing.T) {
	for _, p := range lintAgents(os.DirFS("../..")) {
		t.Error(p)
	}
}

// TestAgentsLintRules plants one violation per rule and checks that it is reported, and that a
// healthy tree reports nothing.
func TestAgentsLintRules(t *testing.T) {
	shim := &fstest.MapFile{Data: []byte("@AGENTS.md\n")}
	long := strings.Repeat("word ", agentsNestedWords+1)
	healthy := func() fstest.MapFS {
		return fstest.MapFS{
			"AGENTS.md":          {Data: []byte("# Root\n\nRun `make verify`. See `internal/AGENTS.md` and `cmd/main.go`.\n")},
			"Makefile":           {Data: []byte("verify:\n\ttrue\n")},
			"cmd/main.go":        {Data: []byte("package main\n")},
			"internal/a.go":      {Data: []byte("package a\n")},
			"internal/AGENTS.md": {Data: []byte("# internal\n\nSee `a.go` and `log/slog`.\n")},
			"internal/CLAUDE.md": shim,
		}
	}
	if got := lintAgents(healthy()); len(got) != 0 {
		t.Fatalf("healthy tree reported %v", got)
	}
	big := healthy()
	for i := range agentsBigDirFiles + 1 {
		big["cmd/f"+strconv.Itoa(i)+".go"] = &fstest.MapFile{Data: []byte("package main\n")}
	}
	cases := []struct {
		name string
		edit func(fstest.MapFS)
		want string
	}{
		{"over budget", func(m fstest.MapFS) { m["internal/AGENTS.md"] = &fstest.MapFile{Data: []byte(long)} }, "words, the limit is 300"},
		{"no shim", func(m fstest.MapFS) { delete(m, "internal/CLAUDE.md") }, "internal/CLAUDE.md must hold"},
		{"missing path", func(m fstest.MapFS) {
			m["internal/AGENTS.md"] = &fstest.MapFile{Data: []byte("# i\n\nSee `internal/gone/x.go`.\n")}
		}, "`internal/gone/x.go` does not exist"},
		{"missing make target", func(m fstest.MapFS) {
			m["internal/AGENTS.md"] = &fstest.MapFile{Data: []byte("# i\n\nRun `make nothing`.\n")}
		}, "make target nothing"},
		{"big dir without AGENTS.md", func(m fstest.MapFS) {
			for k, v := range big {
				m[k] = v
			}
		}, "cmd/ has 27 files and no AGENTS.md"},
		{"sentence repeated from the root", func(m fstest.MapFS) {
			s := "Never skip or disable a check to make it pass; fix the cause or ask."
			m["AGENTS.md"] = &fstest.MapFile{Data: []byte("# Root\n\n" + s + "\n")}
			m["internal/AGENTS.md"] = &fstest.MapFile{Data: []byte("# i\n\n" + s + "\n")}
		}, "repeats a sentence of the root"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := healthy()
			c.edit(m)
			got := strings.Join(lintAgents(m), "\n")
			if !strings.Contains(got, c.want) {
				t.Errorf("want a problem containing %q, got:\n%s", c.want, got)
			}
		})
	}
}
