// Package harness defines the single interface through which every role
// reaches a model. Provider SDKs and CLIs live only under this package.
package harness

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Harness runs one role turn in a workspace and returns a result that matches
// the request's OutputSchema.
type Harness interface {
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

// RunRequest is everything a harness needs for one turn.
type RunRequest struct {
	Role         RoleConfig        // prompt, skills, permissions, limits
	Workspace    string            // worktree path
	Prompt       string            // task prompt for this turn
	OutputSchema json.RawMessage   // JSON Schema the output must satisfy
	SessionID    string            // session to continue, if any
	Budget       Budget            // tokens and minutes for this turn
	Env          map[string]string // only what this provider needs
	// Observe, when set, receives what the harness sees while the turn runs: output, tool calls
	// and usage. The supervisor uses it to stop a stalled or looping run.
	Observe func(Event)
	// SystemFile holds the role and project instructions and the L0 checklist; the adapter
	// adds it to the system prompt. AddDirs hold the skills selected for the task and the read-only standards.
	SystemFile string
	AddDirs    []string
	// ProjectInstructions says SystemFile already holds the project's own instruction files, so
	// the harness must not discover them in the workspace by itself (design section 12). A
	// project that names none keeps the CLI's discovery.
	ProjectInstructions bool
	// Browser, when set, gives the turn a browser tool (a Verifier acceptance job): the only
	// thing the turn writes is the screenshots the tool saves in Browser.EvidenceDir.
	Browser *Browser
}

// Browser is the browser tool of an acceptance turn.
type Browser struct {
	EvidenceDir string // where the tool saves screenshots, and nowhere else
	Config      string // the browser server's configuration file (executable, proxies, flags)
}

// RoleConfig is the part of roles/<name>.yaml a harness needs.
type RoleConfig struct {
	Name     string
	ReadOnly bool
	Skills   []string // skill ids, for example "sentry/code-review"
}

// Budget limits one turn.
type Budget struct {
	Tokens  int
	Minutes int
}

// RunResult is what a harness returns.
type RunResult struct {
	Output     json.RawMessage // already validated against the schema
	SessionID  string
	Transcript string // path in artifacts
	Usage      Usage
	ExitReason ExitReason
}

// Usage is the cost of one turn.
type Usage struct {
	InputTokens  int
	OutputTokens int
	// CacheReadTokens and CacheWriteTokens are the input served from and written to the
	// provider's prompt cache. Claude reports them apart from InputTokens, and no budget counts
	// them: in a recorded Claude run they were about 99% of the input (claude_test.go). Codex
	// reports cached input inside InputTokens, so for it CacheReadTokens is a part of
	// InputTokens (and of its budget) and CacheWriteTokens stays zero.
	CacheReadTokens  int
	CacheWriteTokens int
	CostUSD          float64
	Model            string    // the model the harness reported it ran; empty when it does not say
	LimitHit         bool      // the provider reported a rate or plan limit
	LimitUntil       time.Time // when the limit resets, if the provider said so
}

// ExitReason says why a turn ended.
type ExitReason string

// Exit reasons.
const (
	ExitDone      ExitReason = "done"
	ExitBudget    ExitReason = "budget"
	ExitRateLimit ExitReason = "rate_limit"
	ExitError     ExitReason = "error"
)

// ErrInvalidOutput means the output does not match the requested schema.
var ErrInvalidOutput = errors.New("harness output does not match the schema")
