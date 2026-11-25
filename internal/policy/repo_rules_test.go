package policy_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/policy"
)

// This repository's own rules: whatever protects the factory waits for a person, also when
// low and medium changes merge on their own. Only documents may stay below high.
func TestTheRepositoryHoldsEverythingButDocumentsForAPerson(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile("../../.nightshift/blast-radius.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := policy.ParseRules(doc, config.NewValidator())
	if err != nil {
		t.Fatal(err)
	}
	held := func(path string) bool {
		l := r.Classify(policy.Change{Paths: []string{path}, DiffLines: 1}).Level
		return l == policy.High || l == policy.Critical
	}
	out, err := exec.CommandContext(t.Context(), "git", "-C", "../..", "ls-files", "-z").Output()
	if err != nil {
		t.Fatal(err)
	}
	for path := range strings.SplitSeq(strings.TrimRight(string(out), "\x00"), "\x00") {
		if !strings.HasPrefix(path, "docs/") && !held(path) {
			t.Errorf("%s would merge without a person", path)
		}
	}
	// Files that do not exist yet but would change what is checked or what an agent reads.
	for _, path := range []string{
		"GNUmakefile", "build.mk", ".golangci.json", "mise.local.toml", ".mise.toml", ".tool-versions", "go.work",
		".claude/skills", "docs/CLAUDE.md", "docs/AGENTS.md", "docs/.claude/skills/x/SKILL.md", "docs/x/y.go",
		"internal/new/new.go", "internal/dispatch/a\nb.go", "tools/.hidden", "docs/.golangci.yml",
	} {
		if !held(path) {
			t.Errorf("%q would merge without a person", path)
		}
	}
	if held("docs/runbook.md") {
		t.Error("a short document change must stay low")
	}
}
