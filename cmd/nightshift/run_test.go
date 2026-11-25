package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "no args", args: nil, wantCode: exitUsage, wantStderr: "Usage:"},
		{name: "help", args: []string{"--help"}, wantCode: exitOK, wantStdout: "Commands:"},
		{name: "version", args: []string{"version"}, wantCode: exitOK, wantStdout: "nightshift dev"},
		{name: "skills without a subcommand", args: []string{"skills"}, wantCode: exitUsage, wantStderr: "usage: nightshift skills sync"},
		{name: "work without secrets", args: []string{"work", "--secrets", "/nonexistent"}, wantCode: exitFailure, wantStderr: "worker_token"},
		{name: "status without config", args: []string{"status", "--config", "/nonexistent"}, wantCode: exitFailure, wantStderr: "nightshift status:"},
		{name: "deps without a package", args: []string{"deps", "check"}, wantCode: exitUsage, wantStderr: "usage:"},
		{name: "eval select of core code", args: []string{"eval", "select", "--rules", "../../evals/selection.yaml", "internal/flow/x.go"}, wantCode: exitOK, wantStdout: "no evals"},
		{name: "eval select of a role", args: []string{"eval", "select", "--rules", "../../evals/selection.yaml", "roles/verifier.md"}, wantCode: exitOK, wantStdout: "smoke E1 verifier"},
		{name: "reply without text", args: []string{"reply", "12"}, wantCode: exitUsage, wantStderr: "usage:"},
		{name: "approve without a task", args: []string{"approve"}, wantCode: exitUsage, wantStderr: "usage:"},
		{name: "unhealthy", args: []string{"status", "--health", "--addr", "127.0.0.1:1"}, wantCode: exitUnhealthy, wantStderr: "unhealthy"},
		{name: "serve without config", args: []string{"serve", "--config", "/nonexistent"}, wantCode: exitFailure, wantStderr: "serve failed"},
		{name: "migrate without secrets", args: []string{"serve", "--migrate-only", "--secrets", "/nonexistent"}, wantCode: exitFailure, wantStderr: "pg_owner"},
		{name: "task without a title", args: []string{"task", "add", "example-solo"}, wantCode: exitUsage, wantStderr: "usage: task add"},
		{name: "task without config", args: []string{"task", "add", "example-solo", "--config", "/nonexistent", "do it"}, wantCode: exitFailure, wantStderr: "task add"},
		{name: "chat app link", args: []string{"setup", "chat-app", "--manifest", "../../deploy/slack-manifest.yaml"}, wantCode: exitOK, wantStdout: "new_app=1&manifest_yaml="},
		{name: "chat app without manifest", args: []string{"setup", "chat-app", "--manifest", "/nonexistent"}, wantCode: exitFailure, wantStderr: "chat-app"},
		{name: "unknown", args: []string{"deploy"}, wantCode: exitUsage, wantStderr: `unknown command "deploy"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestEveryCommandIsListedInUsage(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	usage(&out)
	for _, c := range commands {
		if !strings.Contains(out.String(), "  "+c.name+" ") {
			t.Errorf("usage does not list %q", c.name)
		}
	}
}

func TestTenantInitDiscoverWritesDrafts(t *testing.T) {
	t.Parallel()
	repo, out := t.TempDir(), filepath.Join(t.TempDir(), "onboarding")
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"tenant", "init", "demo", "--discover", "--repo-dir", repo, "--out", out}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	for _, f := range []string{"project.yaml", "gaps.md"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	if code := run([]string{"tenant", "init", "../x", "--discover", "--repo-dir", repo}, &stdout, &stderr); code != exitUsage {
		t.Errorf("a bad tenant name gave exit %d", code)
	}
	if code := run([]string{"tenant", "init", "demo"}, &stdout, &stderr); code != exitNotImplemented {
		t.Errorf("init without --discover gave exit %d", code)
	}
}

func TestTenantDoctorReportsAndFails(t *testing.T) {
	t.Parallel()
	cfg, repo := t.TempDir(), t.TempDir()
	copyTo := func(from, to string) {
		t.Helper()
		raw, err := os.ReadFile(from) //nolint:gosec // fixtures of this repository
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to, raw, 0o600); err != nil { //nolint:gosec // a path in the test temp dir
			t.Fatal(err)
		}
	}
	for _, f := range []string{"instance.yaml", "providers.yaml"} {
		copyTo(filepath.Join("..", "..", "testdata", "instances", "personal", f), filepath.Join(cfg, f))
	}
	copyTo(filepath.Join("..", "..", "testdata", "tenants", "example-solo", "tenant.yaml"), filepath.Join(cfg, "tenants", "example-solo", "tenant.yaml"))
	var stdout, stderr bytes.Buffer
	args := []string{"tenant", "doctor", "example-solo", "--config", cfg, "--secrets", t.TempDir(), "--repo-dir", repo}
	if code := run(args, &stdout, &stderr); code != exitFailure {
		t.Fatalf("exit %d, want %d: %s", code, exitFailure, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, "FAIL  project.yaml") || !strings.Contains(out, "FAIL  secret github_token") {
		t.Errorf("report:\n%s", out)
	}
	if code := run([]string{"tenant", "doctor", "nope", "--config", cfg, "--repo-dir", repo}, &stdout, &stderr); code != exitFailure {
		t.Errorf("an unknown tenant gave exit %d", code)
	}
}

func TestEvalSelectReadsPathsWithSpacesFromGit(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...) //nolint:gosec // G204: a fixed program in a test
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "init")
	if err := os.MkdirAll(filepath.Join(dir, "roles"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "roles", "my role.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "change")
	rules, err := filepath.Abs("../../evals/selection.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var out, errb bytes.Buffer
	if code := run([]string{"eval", "select", "--rules", rules, "--base", "HEAD~1"}, &out, &errb); code != exitOK || !strings.Contains(out.String(), "smoke E1 my role") {
		t.Fatalf("code %d, out %q, err %q", code, out.String(), errb.String())
	}
}
