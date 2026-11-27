// Package codex is the Harness adapter for the OpenAI Codex CLI (`codex exec`). It is the
// only place that knows the CLI's flags and event stream (design section 12). With a base URL
// and a key variable it also reaches any provider that speaks the Responses API, for example
// DeepSeek, so one adapter covers both.
package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
)

// Auth is how the CLI signs in.
type Auth string

const (
	// APIKey passes an OpenAI or Responses API key (the default).
	APIKey Auth = "api_key"
	// ChatGPT runs on a ChatGPT plan: a personal access token from the owner's account in
	// CODEX_ACCESS_TOKEN, with no API key in the environment and login forced to ChatGPT, so a
	// run never falls back to paid API usage.
	ChatGPT Auth = "chatgpt"
)

// accessTokenEnv is the variable the CLI reads a ChatGPT personal access token from.
const accessTokenEnv = "CODEX_ACCESS_TOKEN"

// customProvider is the name of the model provider the adapter declares for a base URL.
const customProvider = "nightshift"

// Harness runs `codex exec` in the request workspace.
type Harness struct {
	Bin string // path of the codex binary; defaults to "codex"
	// Auth is api_key (default) or chatgpt; chatgpt takes no BaseURL.
	Auth Auth
	// Model is passed with -m; empty keeps the CLI default.
	Model string
	// Effort is passed as model_reasoning_effort: low, medium, high, xhigh or max; empty keeps
	// the default.
	Effort string
	// BaseURL points the CLI at another Responses API endpoint; empty means OpenAI.
	BaseURL string
	// KeyEnv names the variable in RunRequest.Env that holds the API key; defaults to OPENAI_API_KEY.
	KeyEnv string
	// Sandbox is the CLI sandbox for roles that write. The worker container is the real
	// boundary (ADR 0004), so the worker passes danger-full-access: the CLI's own sandbox
	// needs namespaces the container does not grant. Defaults to workspace-write.
	Sandbox  string
	Home     string // HOME of the agent user, where the CLI keeps its state
	AgentUID uint32 // when set, the CLI runs as this user; the runner is root and must not
	AgentGID uint32
}

// event is the part of one JSONL line of `codex exec --json` we use.
type event struct {
	Type string `json:"type"`
	Item struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Command string `json:"command"`
	} `json:"item"`
	Usage struct {
		InputTokens       int `json:"input_tokens"`
		CachedInputTokens int `json:"cached_input_tokens"` // a part of input_tokens
		OutputTokens      int `json:"output_tokens"`
	} `json:"usage"`
	Message string `json:"message"`
	Error   struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (h *Harness) keyEnv() string {
	if h.KeyEnv != "" {
		return h.KeyEnv
	}
	return "OPENAI_API_KEY"
}

// Args builds the command line for a request. The prompt goes through stdin ("-"), never argv.
func (h *Harness) Args(req harness.RunRequest, schemaFile, lastFile string) []string {
	sandbox := h.Sandbox
	if sandbox == "" {
		sandbox = "workspace-write"
	}
	if req.Role.ReadOnly {
		sandbox = "read-only"
	}
	args := []string{"exec", "--json", "--skip-git-repo-check", "--sandbox", sandbox, "-C", req.Workspace, "-o", lastFile}
	if schemaFile != "" {
		args = append(args, "--output-schema", schemaFile)
	}
	if h.Model != "" {
		args = append(args, "-m", h.Model)
	}
	if h.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+quote(h.Effort))
	}
	if h.Auth == ChatGPT {
		args = append(args, "-c", "forced_login_method="+quote("chatgpt"))
	}
	if h.BaseURL != "" && h.Auth != ChatGPT {
		p := "model_providers." + customProvider
		args = append(args,
			"-c", "model_provider="+quote(customProvider),
			"-c", p+".name="+quote(customProvider),
			"-c", p+".base_url="+quote(h.BaseURL),
			"-c", p+".env_key="+quote(h.keyEnv()),
			"-c", p+".wire_api="+quote("responses"),
		)
	}
	return append(args, "-")
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

// Env builds the child environment from an explicit list. The API key is passed under its
// own name for a custom provider and as CODEX_API_KEY for OpenAI; a chatgpt run gets only the
// access token.
func (h *Harness) Env(req harness.RunRequest) []string {
	env := []string{"PATH=" + os.Getenv("PATH")} //nolint:forbidigo // the CLI must be found; no other variable is passed through
	if h.Home != "" {
		env = append(env, "HOME="+h.Home)
	}
	key := req.Env[h.keyEnv()]
	if h.Auth == ChatGPT {
		key = ""
		if t := req.Env[accessTokenEnv]; t != "" {
			env = append(env, accessTokenEnv+"="+t)
		}
	}
	if key != "" {
		if h.BaseURL == "" {
			env = append(env, "CODEX_API_KEY="+key)
		} else {
			env = append(env, h.keyEnv()+"="+key)
		}
	}
	for _, k := range []string{"HTTPS_PROXY", "NO_PROXY", "TRACEPARENT"} {
		if v := req.Env[k]; v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// exchange makes the temporary directory the runner and the CLI share: the schema the agent
// user reads and the file it writes its final message to. The caller removes dir.
func exchange(outputSchema []byte) (dir, schemaFile, lastFile string, err error) {
	if dir, err = os.MkdirTemp("", "codex-"); err != nil {
		return "", "", "", err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()
	if err = os.Chmod(dir, 0o755); err != nil { //nolint:gosec // the agent user must read the schema
		return dir, "", "", err
	}
	if len(outputSchema) > 0 {
		var schema []byte
		if schema, err = strictSchema(outputSchema); err != nil {
			return dir, "", "", err
		}
		schemaFile = filepath.Join(dir, "schema.json")
		if err = os.WriteFile(schemaFile, schema, 0o644); err != nil { //nolint:gosec // see above
			return dir, "", "", err
		}
	}
	lastFile = filepath.Join(dir, "last.txt")
	if err = os.WriteFile(lastFile, nil, 0o600); err != nil {
		return dir, "", "", err
	}
	// The umask would strip a mode given at creation; the agent user writes the final message here.
	err = os.Chmod(lastFile, 0o666) //nolint:gosec // see above
	return dir, schemaFile, lastFile, err
}

// Run implements harness.Harness.
func (h *Harness) Run(ctx context.Context, req harness.RunRequest) (harness.RunResult, error) {
	if req.Budget.Minutes > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.Budget.Minutes)*time.Minute)
		defer cancel()
	}
	dir, schemaFile, lastFile, err := exchange(req.OutputSchema)
	if err != nil {
		return harness.RunResult{}, fmt.Errorf("codex: %w", err)
	}
	defer os.RemoveAll(dir) //nolint:errcheck // best-effort cleanup of a temp dir

	bin := h.Bin
	if bin == "" {
		bin = "codex"
	}
	cmd := exec.CommandContext(ctx, bin, h.Args(req, schemaFile, lastFile)...) //nolint:gosec // the binary and flags come from the instance configuration
	cmd.Dir = req.Workspace
	cmd.Env = h.Env(req)
	if h.AgentUID != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: h.AgentUID, Gid: h.AgentGID}}
	}
	cmd.Stdin = strings.NewReader(req.Prompt)
	defer harness.StopGroup(cmd)()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = io.MultiWriter(&out, harness.LineWriter(func(line []byte) { observe(req, line) })), &errb
	runErr := cmd.Run()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return harness.RunResult{ExitReason: harness.ExitBudget}, nil
	}
	if ctx.Err() != nil {
		return harness.RunResult{}, fmt.Errorf("codex: %w", ctx.Err())
	}
	rr, failure := parse(out.Bytes())
	if failure == "" && runErr != nil {
		failure = firstLine(errb.String())
		if failure == "" {
			failure = runErr.Error()
		}
	}
	if failure == "" && rr.events == 0 {
		// Without a single event the CLI did not run a turn: an empty answer is not a result.
		failure = "unreadable output: no events"
	}
	rr.ExitReason = harness.ExitDone
	switch {
	case isLimit(failure):
		rr.ExitReason, rr.Usage.LimitHit = harness.ExitRateLimit, true
	case failure != "":
		// Keep the reason: without it a failed job says nothing about why.
		rr.ExitReason, rr.Output = harness.ExitError, errorOutput(failure)
	default:
		final, _ := os.ReadFile(lastFile) //nolint:gosec // a path in our own temp dir
		text := strings.TrimSpace(string(final))
		if text == "" {
			text = rr.last
		}
		rr.Output = output(text, schemaFile != "", req.OutputSchema)
	}
	return rr.RunResult, nil
}

type parsed struct {
	harness.RunResult
	last   string // text of the last agent message
	events int    // lines that were events; none means the output was unreadable
}

// parse reads the event stream: usage from turn.completed, the last agent message, and the
// first failure.
func parse(stream []byte) (parsed, string) {
	var p parsed
	var failure string
	sc := bufio.NewScanner(bytes.NewReader(stream))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var ev event
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Type == "" {
			continue
		}
		p.events++
		switch ev.Type {
		case "item.completed":
			if ev.Item.Type == "agent_message" {
				p.last = ev.Item.Text
			}
		case "turn.completed":
			p.Usage.InputTokens += ev.Usage.InputTokens
			p.Usage.OutputTokens += ev.Usage.OutputTokens
			p.Usage.CacheReadTokens += ev.Usage.CachedInputTokens
		case "turn.failed", "error":
			if failure == "" {
				failure = cmpOr(ev.Error.Message, ev.Message, ev.Type)
			}
		}
	}
	return p, failure
}

func cmpOr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// output returns the result as JSON: the schema-shaped answer as is, plain text wrapped.
func output(text string, schema bool, original []byte) json.RawMessage {
	if schema && json.Valid([]byte(text)) {
		return withoutNulls([]byte(text), original)
	}
	b, _ := json.Marshal(map[string]string{"text": text}) //nolint:errchkjson // a string map always encodes
	return b
}

// maxError is how much of a failure message goes into the result.
const maxError = 1000

func errorOutput(failure string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"error": strings.ToValidUTF8(failure[:min(len(failure), maxError)], "")}) //nolint:errchkjson // a string map always encodes
	return b
}

// isLimit recognizes the messages that mean a rate limit or an exhausted quota.
func isLimit(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "rate limit") || strings.Contains(msg, "usage limit") ||
		strings.Contains(msg, "insufficient_quota") || strings.Contains(msg, "exceeded your current quota")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s[:min(len(s), 300)]
}

// observe turns one event line into supervisor events. Every line is a sign of life; a started
// item that is not a message or reasoning is a tool call, and a finished turn reports usage.
func observe(req harness.RunRequest, line []byte) {
	req.Emit(harness.Event{Kind: harness.EventOutput})
	var ev event
	if json.Unmarshal(line, &ev) != nil {
		return
	}
	switch ev.Type {
	case "item.started":
		if t := ev.Item.Type; t != "agent_message" && t != "reasoning" {
			req.Emit(harness.Event{Kind: harness.EventToolCall, Tool: t, Input: ev.Item.Command})
		}
	case "turn.completed":
		req.Emit(harness.Event{Kind: harness.EventUsage, Usage: harness.Usage{InputTokens: ev.Usage.InputTokens, OutputTokens: ev.Usage.OutputTokens}})
	}
}
