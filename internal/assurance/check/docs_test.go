package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/assurance/check"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

const goodADR = "---\nstatus: proposed\n---\n\n# T\n\n## Context and Problem Statement\n\nx\n\n## Considered Options\n\n- A\n- B\n\n## Decision Outcome\n\nA.\n"

func TestDocFindingsADR(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "docs/adr/0001-good.md", goodADR)
	write(t, dir, "docs/adr/0002-one-option.md", strings.Replace(goodADR, "- B\n", "", 1))
	write(t, dir, "docs/adr/0003-no-front.md", strings.Replace(goodADR, "---\nstatus: proposed\n---\n\n", "", 1))
	write(t, dir, "docs/adr/template.md", "not an ADR file name")
	got, err := check.DocFindings(dir, check.DefaultDocKinds())
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]string{}
	for _, f := range got {
		rules[f.Location.Path] += f.RuleID + " "
		if f.Severity != check.Block || f.Category != "docs" || f.Fingerprint == "" {
			t.Errorf("finding %+v", f)
		}
	}
	if len(rules) != 2 || !strings.Contains(rules["docs/adr/0002-one-option.md"], "adr-alternatives") ||
		!strings.Contains(rules["docs/adr/0003-no-front.md"], "adr-structure") {
		t.Fatalf("findings by file: %v", rules)
	}
}

func TestDocFindingsPRDNeedsGivenWhenThen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prd := "# P\n\n## Problem\n\nx\n\n## Goals\n\nx\n\n## Non-goals\n\nx\n\n## Acceptance Criteria\n\n- the export works\n"
	write(t, dir, "docs/prd/weak.md", prd)
	write(t, dir, "docs/prd/strong.md", strings.Replace(prd, "- the export works", "- Given a report, when exported, then a CSV downloads", 1))
	got, err := check.DocFindings(dir, check.DefaultDocKinds())
	if err != nil || len(got) != 1 || got[0].Location.Path != "docs/prd/weak.md" || got[0].RuleID != "doc-lint.prd-criteria" {
		t.Fatalf("%+v %v", got, err)
	}
}

// The factory's own ADRs follow its rules.
func TestRepositoryADRsPass(t *testing.T) {
	t.Parallel()
	got, err := check.DocFindings("../../..", check.DefaultDocKinds()[:1])
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got {
		t.Errorf("%s: %s", f.Location.Path, f.Message)
	}
}

func TestShippedTemplatesPassTheirOwnLint(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	for _, k := range check.DefaultDocKinds() {
		dir := t.TempDir()
		raw, err := os.ReadFile(filepath.Join(root, "templates", k.Name+".md")) //nolint:gosec // G304: a fixed template path in this repository
		if err != nil {
			t.Fatalf("template for %s: %v", k.Name, err)
		}
		// Put the template where its kind looks for documents.
		path := filepath.Join(dir, strings.Replace(k.Glob, "*", "x", 1))
		path = strings.Replace(path, "[0-9]x", "0001-x", 1)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil { //nolint:gosec // G703: a path under the test's own temporary directory
			t.Fatal(err)
		}
		got, err := check.DocFindings(dir, []check.DocKind{k})
		if err != nil || len(got) != 0 {
			t.Errorf("%s template breaks its own rules: %+v (%v)", k.Name, got, err)
		}
	}
}
