package config

import (
	"strings"
	"testing"
)

// TestARepositoryBelongsToOneTenant: two tenants naming one repository are refused, because
// their parallel loops would share and prune one mirror (T1).
func TestARepositoryBelongsToOneTenant(t *testing.T) {
	t.Parallel()
	inst := Instance{Name: "personal", Tenants: map[string]Tenant{
		"example-solo": {Name: "example-solo", Repos: []string{"github.com/example-owner/example-solo"}},
		"example-team": {Name: "example-team", Repos: []string{"github.com/example-org/example-team", "github.com/example-owner/example-solo"}},
	}}
	err := inst.check()
	if err == nil || !strings.Contains(err.Error(), "tenant example-team: repository github.com/example-owner/example-solo already belongs to tenant example-solo") {
		t.Fatalf("got %v", err)
	}
	inst.Tenants["example-team"] = Tenant{Name: "example-team", Repos: []string{"github.com/example-org/example-team"}}
	if err := inst.check(); err != nil {
		t.Fatalf("distinct repositories refused: %v", err)
	}
}
