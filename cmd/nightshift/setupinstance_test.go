package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/customization/config"
)

func TestSetupWritesAnInstanceThatLoads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var stdout, stderr strings.Builder
	// The repository comes as a flag, the rest from the questions; empty lines keep defaults.
	answers := "\n\nU0OWNER\nC0TASKS\nclaude-sub, codex-api\noff\n2.1.0\n\n"
	code := setupInstance([]string{"--out", dir, "--repo", "example-owner/app"}, strings.NewReader(answers), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	inst, err := config.LoadInstance(config.NewValidator(), filepath.Join(dir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	got := inst.Tenants["example-solo"]
	if got.People.Owner.GitHub != "example-owner" || got.People.Owner.Chat != "U0OWNER" || got.Review.Mode != "off" ||
		strings.Join(got.Providers["builder"], ",") != "claude-sub,codex-api" || got.Chat.Channels["planning"] != "C0TASKS" {
		t.Errorf("tenant: %+v", got)
	}
	env, err := os.ReadFile(filepath.Join(dir, ".env")) //nolint:gosec // a path inside the test temp dir
	if err != nil || !strings.Contains(string(env), "CLAUDE_CODE_VERSION=2.1.0") || strings.Contains(string(env), "CODEX_VERSION") {
		t.Errorf(".env: %q %v", env, err)
	}
	stderr.Reset()
	if code := setupInstance([]string{"--out", dir, "--repo", "example-owner/other", "--ask=false"}, strings.NewReader(""), &stdout, &stderr); code != exitFailure || !strings.Contains(stderr.String(), "nothing was written") {
		t.Errorf("a second run must not overwrite: %d %s", code, stderr.String())
	}
}

func TestSetupRefusesBadAnswers(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][]string{
		"no repository":    nil,
		"repository url":   {"--repo", "https://github.com/o/r"},
		"unknown provider": {"--repo", "o/r", "--providers", "gpt"},
		"half a chat":      {"--repo", "o/r", "--member", "U1"},
		"review mode":      {"--repo", "o/r", "--review", "never"},
		"version":          {"--repo", "o/r", "--claude-version", "latest\nX=1"},
		"tenant":           {"--repo", "o/r", "--tenant", "../x"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			var stdout, stderr strings.Builder
			code := setupInstance(append([]string{"--out", dir, "--ask=false"}, args...), strings.NewReader(""), &stdout, &stderr)
			left, _ := os.ReadDir(dir)
			if code != exitFailure || len(left) != 0 {
				t.Errorf("exit %d, %d file(s) written: %s", code, len(left), stderr.String())
			}
		})
	}
}

func TestSetupWithoutAChatAppUsesTheInbox(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var stdout, stderr strings.Builder
	code := setupInstance([]string{"--out", dir, "--repo", "example-owner/app", "--ask=false"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	inst, err := config.LoadInstance(config.NewValidator(), filepath.Join(dir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	got := inst.Tenants["example-solo"]
	if got.Chat.Adapter != "inbox" || got.People.Owner.Chat == "" {
		t.Errorf("chat: %+v owner: %+v", got.Chat, got.People.Owner)
	}
	if !strings.Contains(stdout.String(), "claude mcp add nightshift -- docker compose exec -T nightshift nightshift mcp") {
		t.Errorf("the one command to connect a Claude client is missing:\n%s", stdout.String())
	}
}
