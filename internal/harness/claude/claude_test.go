package claude_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/harness/claude"
	"github.com/ilyachakun/nightshift/internal/harness/supervise"
)

// These tests are not parallel on purpose: a test that forks while another test still holds
// the write descriptor of its stub makes the exec fail with "text file busy" (ETXTBSY).
//
// stub writes a fake `claude` that records its arguments, stdin and environment, then prints out.
func stub(t *testing.T, out string, exit int) (bin, record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "record")
	script := "#!/bin/sh\n" +
		"{ echo \"ARGS: $*\"; echo \"STDIN: $(cat)\"; env; } > " + record + "\n" +
		"cat <<'JSON'\n" + out + "\nJSON\nexit " + strconv.Itoa(exit) + "\n"
	bin = filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil { //nolint:gosec // a test stub must be executable
		t.Fatal(err)
	}
	return bin, record
}

func request(t *testing.T) harness.RunRequest {
	t.Helper()
	return harness.RunRequest{
		Role: harness.RoleConfig{Name: "builder"}, Workspace: t.TempDir(), Prompt: "fix the bug",
		OutputSchema: []byte(`{"type":"object"}`),
		Env:          map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "sub-token", "ANTHROPIC_API_KEY": "sk-must-not-leak"},
	}
}

func TestRunSuccess(t *testing.T) {
	bin, record := stub(t, `{"type":"result","subtype":"success","is_error":false,"result":"ok","structured_output":{"summary":"done"},"session_id":"s-1","total_cost_usd":0.12,"usage":{"input_tokens":10,"output_tokens":5}}`, 0)
	h := &claude.Harness{Bin: bin, Auth: claude.Subscription, AllowedTools: []string{"Read", "Edit"}}
	req := request(t)
	req.SystemFile, req.AddDirs = "/job/system.md", []string{"/job/skills"}
	res, err := h.Run(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitReason != harness.ExitDone || string(res.Output) != `{"summary":"done"}` || res.SessionID != "s-1" ||
		res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 5 || res.Usage.CostUSD != 0.12 {
		t.Fatalf("result: %+v", res)
	}
	raw, err := os.ReadFile(record) //nolint:gosec // a path inside the test temp dir
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{
		"-p --output-format stream-json --verbose --permission-prompts none --json-schema {\"type\":\"object\"}",
		"--allowedTools Read,Edit", "--append-system-prompt-file /job/system.md", "--add-dir /job/skills", "STDIN: fix the bug", "CLAUDE_CODE_OAUTH_TOKEN=sub-token",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("record lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "--bare") || strings.Contains(got, "sk-must-not-leak") || strings.Contains(got, "ANTHROPIC_API_KEY") {
		t.Errorf("the subscription run saw an API key or --bare:\n%s", got)
	}
}

func TestEnvKeepsOnlyTheCredentialOfItsAuth(t *testing.T) {
	req := request(t)
	api := (&claude.Harness{Auth: claude.APIKey}).Env(req)
	if slices.Contains(api, "CLAUDE_CODE_OAUTH_TOKEN=sub-token") {
		t.Errorf("an API run got the subscription token: %v", api)
	}
	sub := (&claude.Harness{Auth: claude.Subscription}).Env(req)
	if !slices.Contains(sub, "CLAUDE_CODE_OAUTH_TOKEN=sub-token") {
		t.Errorf("the subscription run lost its token: %v", sub)
	}
}

func TestEnvDropsTheKeysOfOtherProviders(t *testing.T) {
	req := request(t)
	req.Env["OPENAI_API_KEY"] = "sk-openai"
	req.Env["DEEPSEEK_API_KEY"] = "sk-deepseek"
	req.Env["HTTPS_PROXY"] = "http://proxy:3128"
	req.Env["OTEL_TRACES_EXPORTER"] = "otlp"
	for _, auth := range []claude.Auth{claude.Subscription, claude.APIKey} {
		env := (&claude.Harness{Auth: auth}).Env(req)
		for _, e := range env {
			if strings.Contains(e, "sk-openai") || strings.Contains(e, "sk-deepseek") {
				t.Errorf("%s: another provider's key reached the agent: %v", auth, env)
			}
		}
		for _, want := range []string{"HTTPS_PROXY=http://proxy:3128", "OTEL_TRACES_EXPORTER=otlp"} {
			if !slices.Contains(env, want) {
				t.Errorf("%s: lost %s: %v", auth, want, env)
			}
		}
	}
}

func TestArgsByAuthAndRole(t *testing.T) {
	req := request(t)
	req.SessionID = "s-9"
	req.Role.ReadOnly = true
	api := &claude.Harness{Auth: claude.APIKey, PermissionMode: "acceptEdits"}
	args := strings.Join(api.Args(req), " ")
	for _, want := range []string{"--bare", "--permission-mode plan", "--resume s-9"} {
		if !strings.Contains(args, want) {
			t.Errorf("args lack %q: %s", want, args)
		}
	}
	if strings.Contains(args, "acceptEdits") {
		t.Errorf("a read-only role got write permissions: %s", args)
	}
	if env := api.Env(req); !slices.Contains(env, "ANTHROPIC_API_KEY=sk-must-not-leak") {
		t.Errorf("the API provider lost its key: %v", env)
	}
}

func TestRunLimitAndErrors(t *testing.T) {
	bin, _ := stub(t, `{"type":"result","subtype":"success","is_error":true,"result":"Claude usage limit reached. Resets at 5pm."}`, 1)
	h := &claude.Harness{Bin: bin, Auth: claude.Subscription}
	res, err := h.Run(t.Context(), request(t))
	if err != nil || res.ExitReason != harness.ExitRateLimit || !res.Usage.LimitHit {
		t.Fatalf("limit: %+v %v", res, err)
	}

	bin, _ = stub(t, `not json`, 1)
	if _, err := (&claude.Harness{Bin: bin}).Run(t.Context(), request(t)); err == nil {
		t.Fatal("unreadable output was accepted")
	}

	bin, _ = stub(t, `{"type":"result","subtype":"error_max_turns","is_error":true,"result":""}`, 1)
	res, err = (&claude.Harness{Bin: bin}).Run(t.Context(), request(t))
	if err != nil || res.ExitReason != harness.ExitBudget {
		t.Fatalf("max turns: %+v %v", res, err)
	}

	bin, _ = stub(t, `{"type":"result","subtype":"error_during_execution","is_error":true,"result":"boom"}`, 1)
	res, err = (&claude.Harness{Bin: bin}).Run(t.Context(), request(t))
	if err != nil || res.ExitReason != harness.ExitError || !strings.Contains(string(res.Output), "error_during_execution: boom") {
		t.Fatalf("a failed job must say why: %+v %v", res, err)
	}
}

func TestPlainTextResultIsWrapped(t *testing.T) {
	bin, _ := stub(t, `{"type":"result","subtype":"success","is_error":false,"result":"hello"}`, 0)
	req := request(t)
	req.OutputSchema = nil
	res, err := (&claude.Harness{Bin: bin}).Run(t.Context(), req)
	if err != nil || string(res.Output) != `{"text":"hello"}` {
		t.Fatalf("result: %+v %v", res, err)
	}
}

// The runner is root, so without an explicit HOME the CLI would have nowhere to keep state.
func TestEnvSetsHomeOfTheAgent(t *testing.T) {
	t.Parallel()
	h := &claude.Harness{Home: "/home/agent"}
	if env := h.Env(harness.RunRequest{}); !slices.Contains(env, "HOME=/home/agent") {
		t.Errorf("no HOME in %v", env)
	}
}

func TestDisallowedToolsReachTheCLI(t *testing.T) {
	h := &claude.Harness{Auth: claude.APIKey, DisallowedTools: []string{"Read", "Bash"}}
	args := h.Args(harness.RunRequest{})
	i := slices.Index(args, "--disallowedTools")
	if i < 0 || args[i+1] != "Read,Bash" {
		t.Fatalf("args %v", args)
	}
}

func TestSupervisorStopsALoopingRunAndItsChildren(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	line := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"go test"}}],"usage":{"input_tokens":3,"output_tokens":2}}}`
	script := "#!/bin/sh\ncat >/dev/null\n" +
		"sleep 60 &\necho $! > " + pidFile + "\n" +
		"for i in 1 2 3 4; do echo '" + line + "'; done\nwait\n"
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil { //nolint:gosec // a test stub must be executable
		t.Fatal(err)
	}
	h := supervise.Supervisor{Policy: supervise.Policy{RepeatLimit: 3}, Next: &claude.Harness{Bin: bin, Auth: claude.Subscription}}
	start := time.Now()
	res, err := h.Run(t.Context(), request(t))
	if err != nil || res.ExitReason != harness.ExitBudget || !strings.Contains(string(res.Output), "loop: Bash") {
		t.Fatalf("res %+v, err %v", res, err)
	}
	if took := time.Since(start); took > 20*time.Second {
		t.Errorf("the stop took %s", took)
	}
	raw, err := os.ReadFile(pidFile) //nolint:gosec // a path inside the test temp dir
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	deadline := time.Now().Add(10 * time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		t.Errorf("the child process %d outlived the stopped run", pid)
	}
}
