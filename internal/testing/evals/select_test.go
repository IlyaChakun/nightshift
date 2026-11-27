package evals_test

import (
	"os"
	"slices"
	"testing"

	"github.com/ilyachakun/nightshift/internal/testing/evals"
)

func rules(t *testing.T) []evals.Rule {
	t.Helper()
	doc, err := os.ReadFile("../../../evals/selection.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := evals.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestACoreCodeChangeTriggersNoEval(t *testing.T) {
	t.Parallel()
	if got := evals.Select(rules(t), []string{"internal/control/flow/move.go", "cmd/nightshift/run.go", "docs/design.md"}); len(got) != 0 {
		t.Fatalf("core code is covered by tests, not evals: %v", got)
	}
}

func TestARolePromptTriggersThatRoleOnly(t *testing.T) {
	t.Parallel()
	got := evals.Select(rules(t), []string{"roles/builder.md"})
	if len(got) != 4 {
		t.Fatalf("builder smoke and fast, E1 and E2: %v", got)
	}
	for _, r := range got {
		if r.Role != "builder" {
			t.Fatalf("only the builder: %v", got)
		}
	}
}

func TestARubricTriggersTheVerifierE1Only(t *testing.T) {
	t.Parallel()
	got := evals.Select(rules(t), []string{"standards/industry/go/style.md"})
	if !slices.Equal(got, []evals.Run{{Set: "fast", Level: "E1", Role: "verifier"}}) {
		t.Fatalf("got %v", got)
	}
}

func TestASkillTriggersBothRolesWithoutDuplicates(t *testing.T) {
	t.Parallel()
	got := evals.Select(rules(t), []string{"skills/own/web-dev/SKILL.md", "skills/own/mobile-dev/SKILL.md"})
	if len(got) != 8 {
		t.Fatalf("two roles, two sets, two levels, no duplicates: %d %v", len(got), got)
	}
}

func TestParseRejectsARuleThatCannotTrigger(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{"- name: x\n  levels: [E1]\n  sets: [fast]\n", "- paths: [a]\n  levels: [E9]\n  sets: [fast]\n", "- paths: [a]\n  levels: [E1]\n  sets: [nightly]\n"} {
		if _, err := evals.Parse([]byte(doc)); err == nil {
			t.Errorf("accepted %q", doc)
		}
	}
}
