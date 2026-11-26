package main

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/harness/claude"
	"github.com/ilyachakun/nightshift/internal/harness/codex"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
)

// TestHarnessesGetTheTenantsOwnHome: the CLI state of claude and codex lives in the HOME of the job's
// tenant, never in the shared agent home (M3).
func TestHarnessesGetTheTenantsOwnHome(t *testing.T) {
	pick := harnessFor("", 20001, 20001)
	const want = "/home/agent/tenants/example-team"
	c, err := pick(jobsapi.Job{Harness: "claude", Auth: "subscription", Tenant: "example-team"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.(*claude.Harness).Home; got != want {
		t.Errorf("claude HOME = %q, want %q", got, want)
	}
	x, err := pick(jobsapi.Job{Harness: "codex", Auth: "api_key", Tenant: "example-team"})
	if err != nil {
		t.Fatal(err)
	}
	if got := x.(*codex.Harness).Home; got != want {
		t.Errorf("codex HOME = %q, want %q", got, want)
	}
}
