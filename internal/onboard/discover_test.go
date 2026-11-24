package onboard_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/onboard"
)

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func TestDiscoverFindsWhatIsThereAndAsksForTheRest(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		repo    fstest.MapFS
		stacks  []string
		verify  string
		instr   []string
		gapWith string
	}{
		"go with a Makefile": {
			repo: fstest.MapFS{
				"go.mod": file("module x\n"), "Makefile": file(".PHONY: verify\nverify: lint test\n\ntest:\n\tgo test ./...\n"),
				"AGENTS.md": file("# Agents\n"), ".golangci.yml": file("version: 2\n"), ".github/workflows/ci.yml": file("on: push\n"),
			},
			stacks: []string{"go"}, verify: "make verify", instr: []string{"AGENTS.md"},
		},
		"python without a test entry point": {
			repo:   fstest.MapFS{"pyproject.toml": file("[project]\n"), "CLAUDE.md": file("x"), ".cursor/rules/style.mdc": file("x")},
			stacks: []string{"python"}, instr: []string{"CLAUDE.md", ".cursor/rules/style.mdc"}, gapWith: "`verify`",
		},
		"ts with an npm test script": {
			repo:   fstest.MapFS{"package.json": file(`{"scripts":{"test":"vitest"}}`), "tsconfig.json": file("{}")},
			stacks: []string{"ts"}, verify: "npm test", gapWith: "Instructions",
		},
		"empty": {repo: fstest.MapFS{}, gapWith: "Stack"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := onboard.Discover(tc.repo, "demo")
			if !slices.Equal(r.Stacks, tc.stacks) || r.Verify != tc.verify || !slices.Equal(r.Instructions, tc.instr) {
				t.Fatalf("stacks %v verify %q instructions %v", r.Stacks, r.Verify, r.Instructions)
			}
			if tc.gapWith != "" && !strings.Contains(strings.Join(r.Gaps, "\n"), tc.gapWith) {
				t.Fatalf("gaps %v lack %q", r.Gaps, tc.gapWith)
			}
			// The decisions that only the owner can make are always asked.
			for _, must := range []string{"Profile", "People", "Blast radius", "Preview"} {
				if !strings.Contains(strings.Join(r.Gaps, "\n"), must) {
					t.Errorf("gaps lack %s", must)
				}
			}
		})
	}
}

func TestDraftsAreReadable(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{"go.mod": file("module x\n"), "AGENTS.md": file("x"), ".nightshift/project.yaml": file("name: x\n")}
	r := onboard.Discover(repo, "demo")
	doc, err := r.ProjectYAML()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: demo", "stack:", "- go", "instructions:", "- AGENTS.md"} {
		if !strings.Contains(string(doc), want) {
			t.Errorf("project.yaml lacks %q:\n%s", want, doc)
		}
	}
	if strings.Contains(string(doc), "verify:") {
		t.Error("a verify command was invented")
	}
	if md := r.GapsMD(); !strings.Contains(md, "already has `.nightshift/project.yaml`") || !strings.Contains(md, "## To ask the owner") {
		t.Errorf("gaps.md:\n%s", md)
	}
}
