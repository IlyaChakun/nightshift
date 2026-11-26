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

// A codex provider on a ChatGPT plan gets the chatgpt login; it cannot point at another
// endpoint, and an unknown auth is refused.
func TestCodexAuth(t *testing.T) {
	pick := harnessFor("", 20001, 20001)
	h, err := pick(jobsapi.Job{Harness: "codex", Auth: "chatgpt", Tenant: "example-solo"})
	if err != nil {
		t.Fatal(err)
	}
	if got := h.(*codex.Harness).Auth; got != codex.ChatGPT {
		t.Errorf("auth = %q, want chatgpt", got)
	}
	for _, job := range []jobsapi.Job{
		{Harness: "codex", Auth: "chatgpt", BaseURL: "https://api.example.test/v1", Tenant: "example-solo"},
		{Harness: "codex", Auth: "subscription", Tenant: "example-solo"},
	} {
		if _, err := pick(job); err == nil {
			t.Errorf("%+v was accepted", job)
		}
	}
}
