// Package claude is the Harness adapter for the Claude Code CLI (`claude -p`). It is the
// only place that knows the CLI's flags and output (design section 12).
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/ilyachakun/nightshift/internal/harness"
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
	Bin            string // path of the claude binary; defaults to "claude"
	Auth           Auth
	PermissionMode string   // for example "acceptEdits"; "plan" for read-only roles
	AllowedTools   []string // explicit tool list; nothing else may run
	SettingsFile   string   // role settings, passed with --settings
	SystemFile     string   // instructions and L0 checklist, passed with --append-system-prompt-file
	AddDirs        []string // job directories with skills and inputs
	AgentUID       uint32   // when set, the CLI runs as this user; the runner is root and must not
	AgentGID       uint32
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
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Args builds the command line for a request. The prompt goes through stdin, never argv.
func (h *Harness) Args(req harness.RunRequest) []string {
	args := []string{"-p", "--output-format", "json", "--permission-prompts", "none"}
	if h.Auth == APIKey {
		args = append(args, "--bare")
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
	if h.SettingsFile != "" {
		args = append(args, "--settings", h.SettingsFile)
	}
	if h.SystemFile != "" {
		args = append(args, "--append-system-prompt-file", h.SystemFile)
	}
	for _, d := range h.AddDirs {
		args = append(args, "--add-dir", d)
	}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	return args
}

// Env builds the child environment from an explicit list: PATH and what the request
// carries. For the subscription, variables that select another way to pay are dropped.
func (h *Harness) Env(req harness.RunRequest) []string {
	env := []string{"PATH=" + os.Getenv("PATH")} //nolint:forbidigo // the CLI must be found; no other variable is passed through
	for k, v := range req.Env {
		if h.Auth == Subscription && slices.Contains(overrides, k) {
			continue
		}
		if h.Auth == APIKey && k == "CLAUDE_CODE_OAUTH_TOKEN" {
			continue
		}
		env = append(env, k+"="+v)
	}
	slices.Sort(env)
	return env
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
	cmd := exec.CommandContext(ctx, bin, h.Args(req)...) //nolint:gosec // the binary and flags come from the instance configuration
	cmd.Dir = req.Workspace
	cmd.Env = h.Env(req)
	if h.AgentUID != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: h.AgentUID, Gid: h.AgentGID}}
	}
	cmd.Stdin = strings.NewReader(req.Prompt)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return harness.RunResult{ExitReason: harness.ExitBudget}, nil
	}
	if ctx.Err() != nil {
		return harness.RunResult{}, fmt.Errorf("claude: %w", ctx.Err())
	}
	var res result
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		if runErr != nil {
			return harness.RunResult{}, fmt.Errorf("claude: %w: %s", runErr, firstLine(errb.String()))
		}
		return harness.RunResult{}, fmt.Errorf("claude: unreadable output: %w", err)
	}
	return toResult(res, runErr), nil
}

func toResult(res result, runErr error) harness.RunResult {
	rr := harness.RunResult{
		SessionID: res.SessionID,
		Usage: harness.Usage{
			InputTokens: res.Usage.InputTokens, OutputTokens: res.Usage.OutputTokens, CostUSD: res.TotalCostUSD,
		},
	}
	switch {
	case isLimit(res):
		rr.ExitReason, rr.Usage.LimitHit = harness.ExitRateLimit, true
	case res.Subtype == "error_max_turns":
		rr.ExitReason = harness.ExitBudget
	case res.IsError || runErr != nil || strings.HasPrefix(res.Subtype, "error"):
		rr.ExitReason = harness.ExitError
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
