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
