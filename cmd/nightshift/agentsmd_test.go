package main

import (
	"io/fs"
	"maps"
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

// skippedDir is data or third-party text the rules do not govern. Only top-level directories
// are checked for a missing AGENTS.md; a large package inside internal/ is the local file's concern.
func skippedDir(name string) bool {
	return name == ".git" || name == "node_modules" || name == "vendor" || name == "testdata" || name == "evals"
}

// agentsTree is what the rules need to know about a tree.
type agentsTree struct {
	agents  []string        // paths of AGENTS.md files
	files   map[string]int  // top-level directory -> file count
	names   map[string]bool // base names of all files
	targets map[string]bool // Makefile targets
}

// scanAgents walks the tree once.
func scanAgents(fsys fs.FS) (agentsTree, error) {
	t := agentsTree{files: map[string]int{}, names: map[string]bool{}, targets: map[string]bool{}}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != "." && skippedDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		t.names[d.Name()] = true
		if top, _, ok := strings.Cut(p, "/"); ok {
			t.files[top]++
		}
		if d.Name() == "AGENTS.md" {
			t.agents = append(t.agents, p)
		}
		return nil
	})
	makeFile, _ := fs.ReadFile(fsys, "Makefile")
	for _, m := range agentsTarget.FindAllStringSubmatch(string(makeFile), -1) {
		t.targets[m[1]] = true
	}
	return t, err
}

// lintAgents returns one problem per violation of the rules above in the tree fsys.
func lintAgents(fsys fs.FS) []string {
	tree, err := scanAgents(fsys)
	var problems []string
	if err != nil {
		problems = append(problems, "cannot read the tree: "+err.Error())
	}
	has := map[string]bool{}
	for _, a := range tree.agents {
		has[path.Dir(a)] = true
	}
	for top, n := range tree.files {
		if n > agentsBigDirFiles && !has[top] {
			problems = append(problems, top+"/ has "+strconv.Itoa(n)+" files and no AGENTS.md; add one with a map, entry points, local rules and how to test")
		}
	}
	rootText, _ := fs.ReadFile(fsys, "AGENTS.md")
	root := sentences(string(rootText))
	for _, a := range tree.agents {
		raw, _ := fs.ReadFile(fsys, a)
		problems = append(problems, lintAgentsFile(fsys, tree, root, a, string(raw))...)
	}
	return problems
}

// lintAgentsFile applies the per-file rules to one AGENTS.md; root holds the root's sentences.
func lintAgentsFile(fsys fs.FS, tree agentsTree, root map[string]string, file, text string) []string {
	dir := path.Dir(file)
	limit := agentsNestedWords
	if dir == "." {
		limit = agentsRootWords
	}
	var problems []string
	if n := len(strings.Fields(text)); n > limit {
		problems = append(problems, file+" has "+strconv.Itoa(n)+" words, the limit is "+strconv.Itoa(limit)+"; cut, do not compress")
	}
	if dir == "." {
		return append(problems, spanProblems(fsys, tree, file, dir, text)...)
	}
	shim, _ := fs.ReadFile(fsys, path.Join(dir, "CLAUDE.md"))
	if strings.TrimSpace(string(shim)) != "@AGENTS.md" {
		problems = append(problems, path.Join(dir, "CLAUDE.md")+" must hold the single line @AGENTS.md: Claude Code loads a nested AGENTS.md only through it")
	}
	for s, orig := range sentences(text) {
		if _, dup := root[s]; dup {
			problems = append(problems, file+" repeats a sentence of the root AGENTS.md; keep it in one place: "+orig)
		}
	}
	return append(problems, spanProblems(fsys, tree, file, dir, text)...)
}

func spanProblems(fsys fs.FS, tree agentsTree, file, dir, text string) []string {
	var problems []string
	for _, m := range agentsSpan.FindAllStringSubmatch(text, -1) {
		problems = append(problems, checkSpan(fsys, tree, file, dir, m[1])...)
	}
	return problems
}

// checkSpan checks one backticked text of an AGENTS.md: a path must exist and every `make`
// target must be in the Makefile. Anything else (commands, placeholders, globs, code) is not checked.
func checkSpan(fsys fs.FS, tree agentsTree, file, dir, span string) []string {
	if agentsMake.MatchString(span) {
		var out []string
		for _, t := range strings.Fields(span)[1:] {
			if !strings.HasPrefix(t, "-") && !strings.Contains(t, "=") && !tree.targets[t] {
				out = append(out, file+": `"+span+"` names the make target "+t+", which the Makefile lacks")
			}
		}
		return out
	}
	p, first, nested, ok := repoPath(span)
	if !ok || (!nested && tree.names[p]) || existsIn(fsys, dir, p, false) {
		return nil
	}
	// Only a first segment that is a real directory marks a repository path: `log/slog` is not one.
	if nested && !agentsFileExt.MatchString(p) && !existsIn(fsys, dir, first, true) {
		return nil
	}
	return []string{file + ": `" + span + "` does not exist"}
}

// repoPath says whether span looks like a path in the repository, and splits it.
func repoPath(span string) (p, first string, nested, ok bool) {
	if strings.ContainsAny(span, " \t") || agentsNotAPath.Replace(span) != span || strings.HasPrefix(span, "http") ||
		strings.HasPrefix(span, "-") || strings.HasPrefix(span, "/") || strings.HasPrefix(span, "~") {
		return "", "", false, false
	}
	p = path.Clean(strings.TrimPrefix(span, "./"))
	first, _, nested = strings.Cut(p, "/")
	return p, first, nested, nested || agentsFileExt.MatchString(p)
}

// existsIn reports whether p exists under the repository root or under dir; onlyDir asks for a directory.
func existsIn(fsys fs.FS, dir, p string, onlyDir bool) bool {
	for _, base := range []string{".", dir} {
		if st, err := fs.Stat(fsys, path.Join(base, p)); err == nil && (!onlyDir || st.IsDir()) {
			return true
		}
	}
	return false
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
			maps.Copy(m, big)
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
