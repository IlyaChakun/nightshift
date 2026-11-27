// Package claude is the Harness adapter for the Claude Code CLI (`claude -p`). It is the
// only place that knows the CLI's flags and output (design section 12).
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
	"github.com/ilyachakun/nightshift/schemas"
)

// Auth says how the CLI pays for a run.
type Auth string

// Auth modes.
const (
	// Subscription uses a token from `claude setup-token`, passed as CLAUDE_CODE_OAUTH_TOKEN
	// in RunRequest.Env. Nothing that would switch the CLI to an API key may reach it.
	Subscription Auth = "subscription"
	// APIKey runs with --bare and ANTHROPIC_API_KEY from RunRequest.Env.
	APIKey Auth = "api_key"
)

// overrides are variables that would make the CLI ignore the subscription.
var overrides = []string{
	"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY",
}

// Harness runs `claude -p` in the request workspace.
type Harness struct {
	Bin             string // path of the claude binary; defaults to "claude"
	Auth            Auth
	Model           string   // passed with --model; empty keeps the CLI default, which changes between versions
	Effort          string   // passed with --effort: low, medium, high, xhigh or max; empty keeps the default
	PermissionMode  string   // for example "acceptEdits"; "plan" for read-only roles
	AllowedTools    []string // explicit tool list; nothing else may run
	DisallowedTools []string // tools that never run, for a turn that must only read its prompt
	SettingsFile    string   // role settings, passed with --settings
	Home            string   // HOME of the agent user, where the CLI keeps its state
	AgentUID        uint32   // when set, the CLI runs as this user; the runner is root and must not
	AgentGID        uint32
}

// result is the part of the CLI's JSON output we use.
type result struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	SessionID        string          `json:"session_id"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
	Usage            struct {
		InputTokens      int `json:"input_tokens"`
		OutputTokens     int `json:"output_tokens"`
		CacheReadTokens  int `json:"cache_read_input_tokens"`
		CacheWriteTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// topLevel are the combinators and conditions the API refuses at the top of the schema it
// gets as a tool input. They stay in the original, against which the result is validated.
var topLevel = []string{"allOf", "anyOf", "oneOf", "not", "if", "then", "else"}

// toolSchema rewrites a result schema into what --json-schema accepts.
func toolSchema(raw []byte) ([]byte, error) {
	flat, err := schemas.Standalone(raw)
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(flat, &doc); err != nil {
		return nil, fmt.Errorf("tool schema: %w", err)
	}
	for _, k := range topLevel {
		delete(doc, k)
	}
	return json.Marshal(doc)
}

// Args builds the command line for a request. The prompt goes through stdin, never argv.
func (h *Harness) Args(req harness.RunRequest) []string {
	args := []string{"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages", "--permission-prompts", "none"}
	if h.Auth == APIKey {
		args = append(args, "--bare")
	}
	if h.Model != "" {
		args = append(args, "--model", h.Model)
	}
	if h.Effort != "" {
		args = append(args, "--effort", h.Effort)
	}
	if len(req.OutputSchema) > 0 {
		args = append(args, "--json-schema", string(req.OutputSchema))
	}
	mode := h.PermissionMode
	if req.Role.ReadOnly {
		mode = "plan"
	}
	if mode != "" {
		args = append(args, "--permission-mode", mode)
	}
	if len(h.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(h.AllowedTools, ","))
	}
	if len(h.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(h.DisallowedTools, ","))
	}
	if h.SettingsFile != "" {
		args = append(args, "--settings", h.SettingsFile)
	}
	if req.SystemFile != "" {
		args = append(args, "--append-system-prompt-file", req.SystemFile)
	}
	for _, d := range req.AddDirs {
		args = append(args, "--add-dir", d)
	}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	return args
}

// Env builds the child environment from an explicit list: PATH, the one credential of this
// auth mode, and the network and telemetry settings the request carries. The worker holds the
// keys of every provider; the agent gets only its own.
func (h *Harness) Env(req harness.RunRequest) []string {
	env := []string{"PATH=" + os.Getenv("PATH")} //nolint:forbidigo // the CLI must be found; no other variable is passed through
	if h.Home != "" {
		env = append(env, "HOME="+h.Home)
	}
	credential := "ANTHROPIC_API_KEY" //nolint:gosec // the name of a variable, not a credential
	if h.Auth == Subscription {
		credential = "CLAUDE_CODE_OAUTH_TOKEN" //nolint:gosec // the name of a variable, not a credential
	}
	if h.Auth == Subscription && req.ProjectInstructions {
		// Without --bare the CLI loads CLAUDE.md from the worktree and its parents, on top of
		// the project instructions the runner already appends (design section 12): the same
		// text twice in every model call. A project that names no instruction files keeps the
		// CLI's own discovery, or it would run with no project rules at all.
		env = append(env, noClaudeMd+"=1")
	}
	for k, v := range req.Env {
		if k == noClaudeMd {
			continue
		}
		if k == credential || passThrough(k) {
			env = append(env, k+"="+v)
		}
	}
	slices.Sort(env)
	return env
}

// noClaudeMd stops the CLI from loading any CLAUDE.md memory file
// (https://code.claude.com/docs/en/env-vars, read 2026-11-26).
const noClaudeMd = "CLAUDE_CODE_DISABLE_CLAUDE_MDS"

// passThrough names the settings that are not credentials: the egress proxy, trace context and
// the CLI's own telemetry switches (compose). Credentials, including the ones that would make
// the CLI ignore the subscription, never pass.
func passThrough(k string) bool {
	switch k {
	case "HTTPS_PROXY", "NO_PROXY", "TRACEPARENT":
		return true
	}
	if slices.Contains(overrides, k) || k == "CLAUDE_CODE_OAUTH_TOKEN" {
		return false
	}
	return strings.HasPrefix(k, "CLAUDE_CODE_") || strings.HasPrefix(k, "OTEL_")
}

// Run implements harness.Harness.
func (h *Harness) Run(ctx context.Context, req harness.RunRequest) (harness.RunResult, error) {
	if req.Budget.Minutes > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.Budget.Minutes)*time.Minute)
		defer cancel()
	}
	bin := h.Bin
	if bin == "" {
		bin = "claude"
	}
	if len(req.OutputSchema) > 0 {
		schema, err := toolSchema(req.OutputSchema)
		if err != nil {
			return harness.RunResult{}, fmt.Errorf("claude: %w", err)
		}
		req.OutputSchema = schema
	}
	cmd := exec.CommandContext(ctx, bin, h.Args(req)...) //nolint:gosec // the binary and flags come from the instance configuration
	cmd.Dir = req.Workspace
	cmd.Env = h.Env(req)
	if h.AgentUID != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: h.AgentUID, Gid: h.AgentGID}}
	}
	cmd.Stdin = strings.NewReader(req.Prompt)
	defer harness.StopGroup(cmd)()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = io.MultiWriter(&out, harness.LineWriter(newObserver(req))), &errb
	runErr := cmd.Run()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return harness.RunResult{ExitReason: harness.ExitBudget, Usage: harness.Usage{Model: initModel(out.Bytes())}}, nil
	}
	if ctx.Err() != nil {
		// The supervisor stops a run by canceling it and keeps this usage: the model must survive.
		return harness.RunResult{Usage: harness.Usage{Model: initModel(out.Bytes())}}, fmt.Errorf("claude: %w", ctx.Err())
	}
	res, err := lastResult(out.Bytes())
	if err != nil {
		if runErr != nil {
			return harness.RunResult{}, fmt.Errorf("claude: %w: %s", runErr, firstLine(errb.String()))
		}
		return harness.RunResult{}, fmt.Errorf("claude: unreadable output: %w", err)
	}
	rr := toResult(res, runErr)
	rr.Usage.Model = initModel(out.Bytes())
	return rr, nil
}

func toResult(res result, runErr error) harness.RunResult {
	rr := harness.RunResult{
		SessionID: res.SessionID,
		Usage: harness.Usage{
			InputTokens: res.Usage.InputTokens, OutputTokens: res.Usage.OutputTokens, CostUSD: res.TotalCostUSD,
			CacheReadTokens: res.Usage.CacheReadTokens, CacheWriteTokens: res.Usage.CacheWriteTokens,
		},
	}
	switch {
	case isLimit(res):
		rr.ExitReason, rr.Usage.LimitHit = harness.ExitRateLimit, true
	case res.Subtype == "error_max_turns":
		rr.ExitReason = harness.ExitBudget
	case res.IsError || runErr != nil || strings.HasPrefix(res.Subtype, "error"):
		// Keep the reason: without it a failed job says nothing about why.
		rr.ExitReason, rr.Output = harness.ExitError, errorOutput(res, runErr)
	default:
		rr.ExitReason = harness.ExitDone
		rr.Output = res.StructuredOutput
		if len(rr.Output) == 0 || string(rr.Output) == "null" {
			// Without --json-schema the result is plain text; wrap it so it is valid JSON.
			rr.Output, _ = json.Marshal(map[string]string{"text": res.Result}) //nolint:errchkjson // a string map always encodes
		}
	}
	return rr
}

// maxError is how much of a failure message goes into the result.
const maxError = 1000

func errorOutput(res result, runErr error) json.RawMessage {
	why := strings.TrimSpace(res.Subtype + ": " + res.Result)
	if runErr != nil {
		why += " (" + runErr.Error() + ")"
	}
	b, _ := json.Marshal(map[string]string{"error": strings.ToValidUTF8(why[:min(len(why), maxError)], "")}) //nolint:errchkjson // a string map always encodes
	return b
}

// isLimit recognizes the messages the CLI prints when a plan or rate limit stops a run.
func isLimit(res result) bool {
	if !res.IsError {
		return false
	}
	msg := strings.ToLower(res.Result)
	return strings.Contains(msg, "usage limit") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "limit reached")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s[:min(len(s), 300)]
}

// lastResult finds the final result object in the stream: the last line of type "result".
func lastResult(stream []byte) (result, error) {
	lines := bytes.Split(stream, []byte("\n"))
	var firstErr error
	for _, raw := range slices.Backward(lines) {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			continue
		}
		var res result
		if err := json.Unmarshal(line, &res); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if res.Type == "result" {
			return res, nil
		}
	}
	if firstErr == nil {
		firstErr = errors.New("no result line")
	}
	return result{}, firstErr
}

// initModel is the model of the stream's system/init line, the model the CLI really runs
// (an alias or a fallback can differ from the one asked for).
func initModel(stream []byte) string {
	for line := range bytes.SplitSeq(stream, []byte("\n")) {
		var l struct{ Type, Subtype, Model string }
		if json.Unmarshal(bytes.TrimSpace(line), &l) == nil && l.Type == "system" && l.Subtype == "init" {
			return l.Model
		}
	}
	return ""
}

// tokens is the usage block of a model message or a partial event.
type tokens struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_input_tokens"`
	CacheWriteTokens int `json:"cache_creation_input_tokens"`
}

// streamLine is the part of one line of the stream we watch. An assistant line carries the
// text and tool calls of one model message. A stream_event line (--include-partial-messages)
// carries one Messages API streaming event: message_start opens a message with its id, and
// message_delta carries that message's cumulative usage
// (https://platform.claude.com/docs/en/build-with-claude/streaming, read 2026-11-26).
type streamLine struct {
	Type    string `json:"type"`
	Parent  string `json:"parent_tool_use_id"`
	Message struct {
		ID      string `json:"id"`
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		Usage tokens `json:"usage"`
	} `json:"message"`
	Event struct {
		Type    string `json:"type"`
		Message struct {
			ID    string `json:"id"`
			Usage tokens `json:"usage"`
		} `json:"message"`
		Usage tokens `json:"usage"`
	} `json:"event"`
}

// newObserver returns the function that turns stream lines into events. Every line is a sign
// of life. Usage is kept per message id as the largest numbers seen, and only the increase is
// emitted. An assistant line repeats a start-of-message snapshot of the output tokens (Mac
// run, 2026-11-26: five messages showed 98 output tokens, the result line 1067), so the real
// count comes from message_delta, matched to its message by the last message_start of the
// same parent tool call. Cache tokens are kept the same way, so a run stopped before its result
// line still records them; no rule counts them.
func newObserver(req harness.RunRequest) func(line []byte) {
	m := &meter{req: req, byID: map[string]tokens{}, open: map[string]string{}}
	return func(line []byte) {
		req.Emit(harness.Event{Kind: harness.EventOutput})
		var sl streamLine
		if json.Unmarshal(line, &sl) == nil {
			m.read(sl)
		}
	}
}

// meter keeps the usage of each message and emits what grows.
type meter struct {
	req  harness.RunRequest
	byID map[string]tokens
	open map[string]string // parent tool call -> id of the message being streamed
}

func (m *meter) read(sl streamLine) {
	switch {
	case sl.Type == "stream_event" && sl.Event.Type == "message_start":
		m.open[sl.Parent] = sl.Event.Message.ID
		m.add(sl.Event.Message.ID, sl.Event.Message.Usage)
	case sl.Type == "stream_event" && sl.Event.Type == "message_delta":
		if id, ok := m.open[sl.Parent]; ok {
			m.add(id, sl.Event.Usage)
		}
	case sl.Type == "assistant":
		for _, c := range sl.Message.Content {
			if c.Type == "tool_use" {
				m.req.Emit(harness.Event{Kind: harness.EventToolCall, Tool: c.Name, Input: string(c.Input)})
			}
		}
		m.add(sl.Message.ID, sl.Message.Usage)
	}
}

func (m *meter) add(id string, u tokens) {
	prev := m.byID[id]
	next := tokens{
		InputTokens: max(prev.InputTokens, u.InputTokens), OutputTokens: max(prev.OutputTokens, u.OutputTokens),
		CacheReadTokens: max(prev.CacheReadTokens, u.CacheReadTokens), CacheWriteTokens: max(prev.CacheWriteTokens, u.CacheWriteTokens),
	}
	if id != "" {
		m.byID[id] = next
	}
	d := harness.Usage{
		InputTokens: next.InputTokens - prev.InputTokens, OutputTokens: next.OutputTokens - prev.OutputTokens,
		CacheReadTokens: next.CacheReadTokens - prev.CacheReadTokens, CacheWriteTokens: next.CacheWriteTokens - prev.CacheWriteTokens,
	}
	if d.InputTokens+d.OutputTokens+d.CacheReadTokens+d.CacheWriteTokens > 0 {
		m.req.Emit(harness.Event{Kind: harness.EventUsage, Usage: d})
	}
}
