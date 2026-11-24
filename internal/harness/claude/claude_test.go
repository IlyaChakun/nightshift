package claude_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/harness/claude"
)

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
	t.Parallel()
	bin, record := stub(t, `{"type":"result","subtype":"success","is_error":false,"result":"ok","structured_output":{"summary":"done"},"session_id":"s-1","total_cost_usd":0.12,"usage":{"input_tokens":10,"output_tokens":5}}`, 0)
	h := &claude.Harness{Bin: bin, Auth: claude.Subscription, AllowedTools: []string{"Read", "Edit"}, SystemFile: "/job/system.md"}
	res, err := h.Run(t.Context(), request(t))
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
		"-p --output-format json --permission-prompts none --json-schema {\"type\":\"object\"}",
		"--allowedTools Read,Edit", "--append-system-prompt-file /job/system.md", "STDIN: fix the bug", "CLAUDE_CODE_OAUTH_TOKEN=sub-token",
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
	t.Parallel()
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

func TestArgsByAuthAndRole(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
}

func TestPlainTextResultIsWrapped(t *testing.T) {
	t.Parallel()
	bin, _ := stub(t, `{"type":"result","subtype":"success","is_error":false,"result":"hello"}`, 0)
	req := request(t)
	req.OutputSchema = nil
	res, err := (&claude.Harness{Bin: bin}).Run(t.Context(), req)
	if err != nil || string(res.Output) != `{"text":"hello"}` {
		t.Fatalf("result: %+v %v", res, err)
	}
}
