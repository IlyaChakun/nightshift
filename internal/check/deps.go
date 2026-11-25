package check

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	shaRe     = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
	reqName   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)
	goModDep  = regexp.MustCompile(`^([A-Za-z0-9][\w.~-]*\.[\w.~-]+(?:/[\w.~-]+)*)\s+v\d`)
	requireIn = regexp.MustCompile(`^require\s*\(\s*$`)
)

// manifestDeps returns the dependency names a manifest declares, by the manifest's file name.
// Names of other files give nil.
func manifestDeps(name, text string, direct bool) map[string]bool {
	switch {
	case name == "go.mod":
		return goModDeps(text, direct)
	case name == "package.json":
		return packageJSONDeps(text)
	case strings.HasSuffix(name, ".txt") && strings.Contains(name, "requirements"):
		return requirementsDeps(text)
	}
	return nil
}

// goModDeps reads the require lines, in blocks and alone; replace, exclude and retract are not
// dependencies. A direct and an indirect requirement both count: moving one between the two
// is not a new dependency. With direct set, indirect requirements are left out: they are what
// `go mod tidy` adds for a direct one.
func goModDeps(text string, direct bool) map[string]bool {
	out := map[string]bool{}
	in := false
	for l := range strings.SplitSeq(text, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case requireIn.MatchString(l):
			in = true
		case l == ")":
			in = false
		case in:
			if m := goModDep.FindStringSubmatch(l); m != nil && (!direct || !strings.Contains(l, "// indirect")) {
				out[m[1]] = true
			}
		case strings.HasPrefix(l, "require "):
			if m := goModDep.FindStringSubmatch(strings.TrimSpace(strings.TrimPrefix(l, "require "))); m != nil && (!direct || !strings.Contains(l, "// indirect")) {
				out[m[1]] = true
			}
		}
	}
	return out
}

func packageJSONDeps(text string) map[string]bool {
	var p map[string]json.RawMessage
	out := map[string]bool{}
	if json.Unmarshal([]byte(text), &p) != nil {
		return out
	}
	for _, section := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
		var m map[string]json.RawMessage
		if json.Unmarshal(p[section], &m) == nil {
			for k := range m {
				out[k] = true
			}
		}
	}
	return out
}

func requirementsDeps(text string) map[string]bool {
	out := map[string]bool{}
	for l := range strings.SplitSeq(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "-") {
			continue
		}
		if n := reqName.FindString(l); n != "" {
			out[strings.ToLower(n)] = true
		}
	}
	return out
}

// DependencyFindings reports every dependency the patch adds to a manifest when no ADR in
// docs/adr lists it under "Considered Options": the research record is missing (design section 6).
// The finding blocks, because a new dependency is blast radius `critical` and the owner needs
// the record to decide. base is the commit the patch starts from. Manifests are found at any depth.
func DependencyFindings(ctx context.Context, dir, base string) ([]Finding, error) {
	if base == "" {
		return nil, nil
	}
	if !shaRe.MatchString(base) {
		return nil, fmt.Errorf("check: dependencies: base %q is not a commit id", base)
	}
	changed, err := git(ctx, dir, "diff", "--name-only", "--no-renames", "--no-ext-diff", "-z", base)
	if err != nil {
		return nil, err
	}
	// The agent's new files are not tracked yet.
	fresh, err := git(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	changed += "\x00" + fresh
	records := adrOptions(dir)
	var out []Finding
	for _, rel := range strings.SplitSeq(strings.TrimRight(changed, "\x00"), "\x00") {
		if rel == "" || !filepath.IsLocal(rel) {
			continue
		}
		name := path.Base(rel)
		if manifestDeps(name, "", false) == nil {
			continue
		}
		head, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))) //nolint:gosec // G304: a path git listed inside the worktree
		if err != nil {
			continue // the patch deleted the manifest
		}
		old, _ := git(ctx, dir, "show", base+":"+rel) // a manifest the patch adds has no base version
		before := manifestDeps(name, old, false)
		for dep := range manifestDeps(name, string(head), true) {
			if !before[dep] && !named(records, dep) {
				out = append(out, depFinding(rel, dep))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Location.Path != out[j].Location.Path {
			return out[i].Location.Path < out[j].Location.Path
		}
		return out[i].Message < out[j].Message
	})
	return out, nil
}

func depFinding(p, name string) Finding {
	f := Finding{
		RuleID: "dependency.no-research-record", Category: "process", Severity: Block, LayerCaught: "L2",
		Message: "the patch adds the dependency " + name + " and no ADR in docs/adr lists it under \"Considered Options\"; stop with needs_research, " +
			"so Lead records the choice (build-vs-buy, options including writing it ourselves)",
		Location: &Location{Path: p},
	}
	f.Fingerprint = fingerprint(f, nil)
	return f
}

// adrOptions is the "Considered Options" text of every ADR of the worktree.
func adrOptions(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "docs", "adr", "*.md"))
	var b strings.Builder
	for _, f := range files {
		if raw, err := os.ReadFile(f); err == nil { //nolint:gosec // G304: a glob of the worktree's ADRs
			b.WriteString(section(string(raw), "Considered Options"))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// named reports whether text mentions the package as a whole word, so `example.com/new` is not
// found in `example.com/newer`.
func named(text, name string) bool {
	re, err := regexp.Compile(`(?i)(^|[^\w./@-])` + regexp.QuoteMeta(name) + `($|[^\w@-]|[./-]\s|[./-]$)`)
	return err == nil && re.MatchString(text)
}

// git runs one read-only git command with the settings of the checkout ignored: the patch is
// the agent's, and its .git/config or .gitattributes must not run a program here.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "safe.directory=*"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...) //nolint:gosec // G204: fixed program; the arguments are built here and the base is checked to be a commit id
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/nonexistent", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"} //nolint:forbidigo // git needs the PATH, nothing else
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("check: git %s: %w: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
