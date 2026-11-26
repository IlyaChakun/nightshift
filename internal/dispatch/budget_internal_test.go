package dispatch

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
)

// The tenant's budget.run goes over the role's limits key by key; a key it leaves out keeps the role's.
func TestTenantRunLimitsOverrideTheRole(t *testing.T) {
	t.Parallel()
	role := config.Role{Limits: config.RoleLimits{Minutes: 45, Tokens: 600000, RepeatLimit: new(4)}}

	got := (&Conductor{}).budget(role)
	if got.Minutes != 45 || got.Tokens != 600000 || got.RepeatLimit == nil || *got.RepeatLimit != 4 || got.StallMinutes != nil || got.CostUSD != nil {
		t.Fatalf("role only: %+v", got)
	}

	c := &Conductor{Run: config.RoleLimits{Minutes: 90, StallMinutes: new(0), CostUSD: new(5.0)}}
	got = c.budget(role)
	if got.Minutes != 90 || got.Tokens != 600000 || *got.RepeatLimit != 4 || got.StallMinutes == nil || *got.StallMinutes != 0 || *got.CostUSD != 5.0 {
		t.Fatalf("with tenant: %+v", got)
	}
}

// A provider's own model is a pin; otherwise the tenant's model for the harness wins over the
// role's, and the tenant's effort over the role's.
func TestModelOfLayersProviderTenantAndRole(t *testing.T) {
	t.Parallel()
	lead := config.Role{Name: "lead", Model: map[string]string{"claude": "claude-opus-5-5"}, Effort: "high"}
	sub := config.Provider{Harness: "claude"}
	pinned := config.Provider{Harness: "codex", Model: "example-model-1"}
	tenant := &Conductor{RoleRuns: map[string]config.RoleRun{"lead": {Model: map[string]string{"claude": "claude-sonnet-5-5"}, Effort: "max"}}}
	for _, tc := range []struct {
		name          string
		c             *Conductor
		prov          config.Provider
		role          config.Role
		model, effort string
	}{
		{"role default", &Conductor{}, sub, lead, "claude-opus-5-5", "high"},
		{"tenant over role", tenant, sub, lead, "claude-sonnet-5-5", "max"},
		{"provider pin over both", tenant, pinned, lead, "example-model-1", "max"},
		{"no model for this harness", &Conductor{}, config.Provider{Harness: "codex"}, lead, "", "high"},
		{"a role without settings", &Conductor{}, sub, config.Role{Name: "builder"}, "", ""},
	} {
		if m, e := tc.c.modelOf(tc.role, tc.prov); m != tc.model || e != tc.effort {
			t.Errorf("%s: got %q %q, want %q %q", tc.name, m, e, tc.model, tc.effort)
		}
	}
}
