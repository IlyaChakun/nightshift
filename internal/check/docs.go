package check

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DocKind ties the documents under one glob to the rules they must follow (design section 8,
// doc-lint). Globs are relative to the worktree and match with filepath.Match.
type DocKind struct {
	Name  string
	Glob  string
	Rules DocRules
	// Options is the least number of list items under "Considered Options" (an ADR weighs at
	// least two alternatives); 0 skips the rule.
	Options int
	// Criteria asks for Given, When and Then in the "Acceptance Criteria" section.
	Criteria bool
}

// DefaultDocKinds are the documents the factory itself writes: ADRs (MADR 4.0) and PRDs.
var DefaultDocKinds = []DocKind{
	{
		Name: "adr", Glob: "docs/adr/[0-9]*.md", Options: 2,
		Rules: DocRules{Front: true, Sections: []string{"Context and Problem Statement", "Considered Options", "Decision Outcome"}},
	},
	{
		Name: "prd", Glob: "docs/prd/*.md", Criteria: true,
		Rules: DocRules{Sections: []string{"Problem", "Goals", "Non-goals", "Acceptance Criteria"}, MaxLines: 150},
	},
}

var listItem = regexp.MustCompile(`^\s*([-*]|\d+\.)\s+\S`)

// DocFindings lints the documents of the worktree that match the kinds and returns one block
// finding per violation. The message tells the author how to fix it.
func DocFindings(dir string, kinds []DocKind) ([]Finding, error) {
	var out []Finding
	for _, k := range kinds {
		files, err := filepath.Glob(filepath.Join(dir, k.Glob))
		if err != nil {
			return nil, fmt.Errorf("check: doc-lint %s: %w", k.Name, err)
		}
		for _, file := range files {
			raw, err := os.ReadFile(file) //nolint:gosec // G304: the path comes from a glob of the worktree
			if err != nil {
				return nil, fmt.Errorf("check: doc-lint: %w", err)
			}
			rel, _ := filepath.Rel(dir, file)
			rel = filepath.ToSlash(rel)
			text := string(raw)
			for _, v := range DocLint(text, k.Rules) {
				out = append(out, docFinding(k, rel, "structure", v.Line, v.Msg))
			}
			if k.Options > 0 {
				if n := countItems(section(text, "Considered Options")); n < k.Options {
					out = append(out, docFinding(k, rel, "alternatives", 0,
						fmt.Sprintf("list at least %d alternatives under \"Considered Options\"; it has %d", k.Options, n)))
				}
			}
			if k.Criteria {
				body := strings.ToLower(section(text, "Acceptance Criteria"))
				for _, word := range []string{"given", "when", "then"} {
					if !strings.Contains(body, word) {
						out = append(out, docFinding(k, rel, "criteria", 0,
							"write each acceptance criterion as Given, When, Then; the section lacks \""+word+"\""))
						break
					}
				}
			}
		}
	}
	return out, nil
}

func docFinding(k DocKind, path, rule string, line int, msg string) Finding {
	f := Finding{
		RuleID: "doc-lint." + k.Name + "-" + rule, Category: "docs", Severity: Block, LayerCaught: "L1",
		Message:  msg + " (" + k.Name + ", see its template under templates/)",
		Location: &Location{Path: path},
	}
	if line > 0 {
		f.Location.StartLine = line
	}
	f.Fingerprint = fingerprint(f, nil)
	return f
}

// section returns the body of a level-2 section, up to the next level-2 heading.
func section(text, name string) string {
	var b strings.Builder
	in := false
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "## ") {
			in = strings.TrimSpace(l[3:]) == name
			continue
		}
		if in {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

func countItems(body string) int {
	n := 0
	for _, l := range strings.Split(body, "\n") {
		if listItem.MatchString(l) {
			n++
		}
	}
	return n
}
