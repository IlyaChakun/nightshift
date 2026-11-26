package main

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
)

func TestARoleRunForNoRoleIsRefused(t *testing.T) {
	t.Parallel()
	roles := map[string]config.Role{"lead": {Name: "lead"}}
	ok := config.Tenant{Name: "example-solo", Roles: map[string]config.RoleRun{"lead": {Effort: "max"}}}
	if err := checkRoleRuns(map[string]config.Tenant{"example-solo": ok}, roles); err != nil {
		t.Fatal(err)
	}
	typo := config.Tenant{Name: "example-solo", Roles: map[string]config.RoleRun{"leed": {Effort: "max"}}}
	if err := checkRoleRuns(map[string]config.Tenant{"example-solo": typo}, roles); err == nil || !strings.Contains(err.Error(), "roles.leed") {
		t.Fatalf("a typo was accepted: %v", err)
	}
}
