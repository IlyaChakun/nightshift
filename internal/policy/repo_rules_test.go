package policy_test

import (
	"os"
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/policy"
)

// This repository's own rules: whatever protects the factory waits for a person, also when
// low and medium changes merge on their own.
func TestTheRepositoryHoldsItsProtectionsForAPerson(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile("../../.nightshift/blast-radius.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := policy.ParseRules(doc, config.NewValidator())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		".nightshift/blast-radius.yaml", ".github/workflows/verify.yml", "deploy/egress-acl.yaml", "schemas/tenant.json",
		"internal/policy/review.go", "internal/dispatch/gate.go", "internal/check/run.go", "internal/flow/flow.go",
		"internal/runner/check.go", "internal/toolchain/toolchain.go", "internal/config/secrets.go",
		"internal/adapters/github/github.go", "internal/jobsapi/server.go", "cmd/nightshift/serve.go",
		"internal/gitops/gitops.go", "internal/book/queries/tasks.sql", "roles/verifier.md", "scripts/repo-check.sh",
		".golangci.yml", ".agents/skills/finish-task/SKILL.md", "standards/industry/go/style.md",
	} {
		got := r.Classify(policy.Change{Paths: []string{path}, DiffLines: 1})
		if got.Level != policy.High && got.Level != policy.Critical {
			t.Errorf("%s is %s", path, got.Level)
		}
		// A test file beside it must not lower the change.
		mixed := r.Classify(policy.Change{Paths: []string{"docs/runbook.md", path}, DiffLines: 1})
		if mixed.Level != got.Level {
			t.Errorf("%s with a document is %s", path, mixed.Level)
		}
	}
	if got := r.Classify(policy.Change{Paths: []string{"docs/runbook.md"}, DiffLines: 5}); got.Level != policy.Low {
		t.Errorf("a short document change is %s", got.Level)
	}
}
