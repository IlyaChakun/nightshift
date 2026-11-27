package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/customization/onboard"
)

// A fresh machine: every line says what is missing, and nothing prints a secret.
func TestDoctorOnAnEmptyInstance(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	checks, closeBook := serviceChecks(t.TempDir(), config.Secrets{Dir: t.TempDir()})
	defer closeBook()
	if onboard.RunChecks(t.Context(), &out, checks) {
		t.Errorf("an empty instance passed:\n%s", out.String())
	}
	for _, want := range []string{"fail  config", "fail  book", "docs/setup.md"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}

func TestWorkerChecks(t *testing.T) {
	t.Parallel()
	secrets := t.TempDir()
	if err := os.MkdirAll(filepath.Join(secrets, "harness"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secrets, "harness", "OPENAI_API_KEY"), []byte("test-only-value"), 0o400); err != nil {
		t.Fatal(err)
	}
	look := func(bin string) (string, error) {
		if bin == "codex" {
			return "/bin/codex", nil
		}
		return "", errors.New("missing")
	}
	var out strings.Builder
	onboard.RunChecks(t.Context(), &out, workerChecks(config.Secrets{Dir: secrets}, "", look))
	got := out.String()
	for _, want := range []string{"ok    agent CLIs: codex", "ok    model credentials: OPENAI_API_KEY", "fail  egress proxy"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "test-only-value") {
		t.Error("the report holds a credential")
	}
}

type hosts map[string]int

func (h hosts) Workflows(_ context.Context, repo string) (int, error) {
	n, ok := h[repo]
	if !ok {
		return 0, errors.New("not installed")
	}
	return n, nil
}

func TestRepositoryChecks(t *testing.T) {
	t.Parallel()
	tenant := config.Tenant{Name: "example-solo", Repos: []string{"github.com/o/ready", "github.com/o/no-ci", "github.com/o/unreachable"}}
	var out strings.Builder
	onboard.RunChecks(t.Context(), &out, repositoryChecks(tenant, hosts{"o/ready": 1, "o/no-ci": 0}))
	for _, want := range []string{"ok    example-solo repository o/ready", "fail  example-solo repository o/no-ci: no CI workflow", "fail  example-solo repository o/unreachable: not installed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}

// A tenant without a chat app uses the inbox in the book; the doctor called it unknown.
func TestInboxChatPassesTheDoctor(t *testing.T) {
	t.Parallel()
	tenant := config.Tenant{Name: "example-solo", Chat: config.Adapter{Adapter: "inbox"}}
	var out strings.Builder
	onboard.RunChecks(t.Context(), &out, tenantChecks(&config.Instance{}, tenant, config.Secrets{Dir: t.TempDir()})[1:])
	if !strings.Contains(out.String(), "ok    example-solo chat: inbox") {
		t.Errorf("report:\n%s", out.String())
	}
}
