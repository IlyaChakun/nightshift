package integration

import (
	"encoding/json"
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
)

// TestFindingMatchesSchema keeps the Go type and schemas/finding.json in sync.
func TestFindingMatchesSchema(t *testing.T) {
	t.Parallel()
	f := Finding{
		RuleID:        "go.errors.wrap",
		Category:      "code",
		Severity:      "block",
		LayerCaught:   "L3",
		LayerExpected: "L1",
		Message:       "Error returned without context.",
		Location:      &Location{Path: "main.go", StartLine: 3},
		Evidence:      []string{"artifacts/x.sarif"},
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.NewValidator().ValidateJSON(config.KindFinding, raw); err != nil {
		t.Error(err)
	}
}
