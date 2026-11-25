package check

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// manifests are the files whose added lines name a new dependency, with how to read one.
var manifests = []struct {
	glob string
	line *regexp.Regexp
}{
	{"go.mod", regexp.MustCompile(`^\+\s*(?:require\s+)?([A-Za-z0-9][\w./~-]*\.[\w./~-]+)\s+v\d\S*\s*$`)},
	{"package.json", regexp.MustCompile(`^\+\s*"(@?[a-z0-9][\w./-]*)":\s*"[\^~<>=*\d][^"]*"`)},
	{"requirements*.txt", regexp.MustCompile(`^\+([A-Za-z][\w.-]*)\s*(?:[=<>~!]=?|$)`)},
}

// notDependencies are package.json keys that look like a dependency but are not.
var notDependencies = map[string]bool{"version": true, "node": true, "npm": true}

// DependencyFindings reports every dependency the patch adds to a manifest when no ADR in
// docs/adr names it: the research record is missing (design section 6). The finding blocks,
// because a new dependency is blast radius `critical` and the owner needs the record to decide.
// base is the commit the patch starts from.
func DependencyFindings(ctx context.Context, dir, base string) ([]Finding, error) {
	if base == "" {
		return nil, nil
	}
	records := adrText(dir)
	var out []Finding
	for _, m := range manifests {
		files, err := filepath.Glob(filepath.Join(dir, m.glob))
		if err != nil {
			return nil, fmt.Errorf("check: dependencies: %w", err)
		}
		for _, file := range files {
			rel, _ := filepath.Rel(dir, file)
			diff, err := gitDiff(ctx, dir, base, rel)
			if err != nil {
				return nil, err
			}
			for _, name := range addedDeps(diff, m.line) {
				if !strings.Contains(records, strings.ToLower(name)) {
					out = append(out, depFinding(rel, name))
				}
			}
		}
	}
	return out, nil
}

// addedDeps returns the names on added lines that no removed line names: a line that only
// moved or gained a comma is not a new dependency.
func addedDeps(diff string, re *regexp.Regexp) []string {
	removed := map[string]bool{}
	var added []string
	for l := range strings.SplitSeq(diff, "\n") {
		if strings.Contains(l, "// indirect") {
			continue
		}
		if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			if m := re.FindStringSubmatch("+" + l[1:]); m != nil {
				removed[m[1]] = true
			}
		}
		if m := re.FindStringSubmatch(l); m != nil && !notDependencies[m[1]] {
			added = append(added, m[1])
		}
	}
	return slices.DeleteFunc(added, func(n string) bool { return removed[n] })
}

func depFinding(path, name string) Finding {
	f := Finding{
		RuleID: "dependency.no-research-record", Category: "process", Severity: Block, LayerCaught: "L2",
		Message: "the patch adds the dependency " + name + " and no ADR in docs/adr names it; stop with needs_research, " +
			"so Lead records the choice (build-vs-buy, options including writing it ourselves)",
		Location: &Location{Path: path},
	}
	f.Fingerprint = fingerprint(f, nil)
	return f
}

// adrText is the lower-cased text of every ADR of the worktree.
func adrText(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "docs", "adr", "*.md"))
	var b strings.Builder
	for _, f := range files {
		if raw, err := os.ReadFile(f); err == nil { //nolint:gosec // G304: a glob of the worktree's ADRs
			b.WriteString(strings.ToLower(string(raw)))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func gitDiff(ctx context.Context, dir, base, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--no-color", "-U0", base, "--", path)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("check: git diff %s: %w: %s", path, err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
