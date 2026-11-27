package main

import (
	"os"
	"testing"

	"github.com/ilyachakun/nightshift/internal/assurance/policy"
	"github.com/ilyachakun/nightshift/internal/customization/config"
)

// The repository is itself a project of an instance; a typo in these files would only show
// when a task runs.
func TestOwnProjectFilesAreValid(t *testing.T) {
	t.Parallel()
	v := config.NewValidator()
	project, err := os.ReadFile("../../.nightshift/project.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := v.ValidateYAML(config.KindProject, project); err != nil {
		t.Error(err)
	}
	raw, err := os.ReadFile("../../.nightshift/blast-radius.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := policy.ParseRules(raw, v)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]policy.Level{"deploy/docker-compose.yml": policy.Critical, "internal/execution/runner/runner.go": policy.High, "internal/observability/summary/summary.go": policy.High, "docs/runbook.md": policy.Low} {
		if got := rules.Classify(policy.Change{Paths: []string{path}, DiffLines: 10}).Level; got != want {
			t.Errorf("%s: %s, want %s", path, got, want)
		}
	}
}
