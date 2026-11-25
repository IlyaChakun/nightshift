package codex_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/harness/codex"
)

// stub writes a fake `codex` that records arguments, stdin and environment, writes the
// final message to the -o file, and prints the event stream.
func stub(t *testing.T, events, last string, exit int) (bin, record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "record")
	script := "#!/bin/sh\n" +
		"{ echo \"ARGS: $*\"; echo \"STDIN: $(cat)\"; env; } > " + record + "\n" +
		"while [ $# -gt 0 ]; do [ \"$1\" = -o ] && printf '%s' '" + last + "' > \"$2\"; shift; done\n" +
		"cat <<'JSON'\n" + events + "\nJSON\nexit " + strconv.Itoa(exit) + "\n"
	bin = filepath.Join(dir, "codex")
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
		Env:          map[string]string{"OPENAI_API_KEY": "sk-openai", "DEEPSEEK_API_KEY": "sk-ds", "TRACEPARENT": "00-a-b-01", "HTTPS_PROXY": "http://proxy.test:1"},
	}
}

const done = `{"type":"thread.started","thread_id":"t-1"}
{"type":"item.completed","item":{"type":"agent_message","text":"ignored"}}
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":4,"output_tokens":5}}`

func record(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // a path inside the test temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestRunOpenAI(t *testing.T) {
	t.Parallel()
	bin, rec := stub(t, done, `{"summary":"done"}`, 0)
	h := &codex.Harness{Bin: bin, Model: "gpt-test", Sandbox: "danger-full-access", Home: "/home/agent"}
	res, err := h.Run(t.Context(), request(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitReason != harness.ExitDone || string(res.Output) != `{"summary":"done"}` ||
		res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 5 {
		t.Fatalf("result: %+v", res)
	}
	got := record(t, rec)
	for _, want := range []string{
		"exec --json --skip-git-repo-check --sandbox danger-full-access", "--output-schema", "-m gpt-test",
		"STDIN: fix the bug", "CODEX_API_KEY=sk-openai", "HOME=/home/agent", "TRACEPARENT=00-a-b-01", "HTTPS_PROXY=http://proxy.test:1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("record lacks %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"sk-ds", "model_providers"} {
		if strings.Contains(got, bad) {
			t.Errorf("record has %q:\n%s", bad, got)
		}
	}
}

func TestRunCustomProvider(t *testing.T) {
	t.Parallel()
	bin, rec := stub(t, done, `{}`, 0)
	h := &codex.Harness{Bin: bin, BaseURL: "https://api.example.test/v1", KeyEnv: "DEEPSEEK_API_KEY", Model: "m"}
	if _, err := h.Run(t.Context(), request(t)); err != nil {
		t.Fatal(err)
	}
	got := record(t, rec)
	for _, want := range []string{
		`model_providers.nightshift.base_url="https://api.example.test/v1"`,
		`model_providers.nightshift.env_key="DEEPSEEK_API_KEY"`, `model_provider="nightshift"`,
		"DEEPSEEK_API_KEY=sk-ds",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("record lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sk-openai") || strings.Contains(got, "CODEX_API_KEY") {
		t.Errorf("OpenAI key leaked to a custom provider:\n%s", got)
	}
}

func TestReadOnlyRole(t *testing.T) {
	t.Parallel()
	bin, rec := stub(t, done, `{}`, 0)
	req := request(t)
	req.Role.ReadOnly = true
	if _, err := (&codex.Harness{Bin: bin, Sandbox: "danger-full-access"}).Run(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if got := record(t, rec); !strings.Contains(got, "--sandbox read-only") {
		t.Errorf("read-only role must use the read-only sandbox:\n%s", got)
	}
}

func TestFailures(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		events string
		exit   int
		want   harness.ExitReason
		limit  bool
		output string // the failure is kept, so a failed job says why
	}{
		"rate limit":   {`{"type":"turn.failed","error":{"message":"Rate limit reached"}}`, 1, harness.ExitRateLimit, true, ""},
		"quota":        {`{"type":"error","message":"insufficient_quota"}`, 1, harness.ExitRateLimit, true, ""},
		"other":        {`{"type":"turn.failed","error":{"message":"boom"}}`, 1, harness.ExitError, false, `{"error":"boom"}`},
		"silent crash": {``, 1, harness.ExitError, false, `{"error":"exit status 1"}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bin, _ := stub(t, tc.events, ``, tc.exit)
			res, err := (&codex.Harness{Bin: bin}).Run(t.Context(), request(t))
			if err != nil {
				t.Fatal(err)
			}
			if res.ExitReason != tc.want || res.Usage.LimitHit != tc.limit || string(res.Output) != tc.output {
				t.Fatalf("result: %+v", res)
			}
		})
	}
}

func TestPlainTextWithoutSchema(t *testing.T) {
	t.Parallel()
	bin, _ := stub(t, done, `hello`, 0)
	req := request(t)
	req.OutputSchema = nil
	res, err := (&codex.Harness{Bin: bin}).Run(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Output) != `{"text":"hello"}` {
		t.Fatalf("output: %s", res.Output)
	}
}
