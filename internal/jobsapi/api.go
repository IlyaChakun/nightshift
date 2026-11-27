// Package jobsapi is the data-only job protocol between nightshift and the worker (and the
// read-only ns CLI): routes, request and result messages, the patch artifact's name, and the
// worker's HTTP client. The worker never talks to Postgres; it claims a job, reports progress and
// completes it over this API (design section 3). Package jobserver serves it over the book.
package jobsapi

import (
	"encoding/json"
	"slices"
	"time"
)

// Routes of the jobs API. {id} is the job id.
const (
	RouteClaim     = "POST /jobs/claim"          // long-poll; returns a job and its token
	RouteHeartbeat = "POST /jobs/{id}/heartbeat" // extends the job lease
	RouteEvents    = "POST /jobs/{id}/events"    // appends progress events
	RouteComplete  = "POST /jobs/{id}/complete"  // result, patch path, artifact hashes

	RouteSearch    = "GET /ns/search"    // knowledge search within the role's visibility
	RouteStandards = "GET /ns/standards" // standards that apply to the job
	RouteTask      = "GET /ns/task"      // the task as the role may see it
)

// PatchName is the file name of the artifact that carries a job's changes: the worker writes it
// and the server and dispatch find the patch by it.
const PatchName = "patch.diff"

// MaxBody caps one request or response body of the jobs API, on both sides.
const MaxBody = 4 << 20

// Auth: the worker authenticates RouteClaim with its worker token. Every
// other route takes the job token from ClaimResponse in the Authorization
// header; it is bound to one task and role and expires with the job.

// ClaimRequest asks for the next job.
type ClaimRequest struct {
	WorkerID string        `json:"worker_id"`
	Wait     time.Duration `json:"wait"` // long-poll timeout
}

// ClaimResponse carries one job. An empty JobID means no job arrived in time.
type ClaimResponse struct {
	JobID     string    `json:"job_id"`
	JobToken  string    `json:"job_token"`
	ExpiresAt time.Time `json:"expires_at"`
	Job       Job       `json:"job"`
}

// Job is the role's view of a task: only the data that role may see.
// Builder jobs never carry hidden scenarios.
type Job struct {
	Tenant       string          `json:"tenant"`
	TaskID       string          `json:"task_id"`
	Role         string          `json:"role"`
	Provider     string          `json:"provider"`
	Harness      string          `json:"harness"`            // from the provider: claude, fake, ...
	Auth         string          `json:"auth"`               // from the provider: subscription, api_key, ...
	Model        string          `json:"model,omitempty"`    // the provider's, else the role's or the tenant's for this harness
	Effort       string          `json:"effort,omitempty"`   // the role's reasoning effort, or the tenant's
	BaseURL      string          `json:"base_url,omitempty"` // from the provider: an OpenAI-compatible endpoint
	KeyEnv       string          `json:"key_env,omitempty"`  // from the provider: the credential file and variable name
	Price        *Price          `json:"price,omitempty"`    // from the provider: prices tokens when the harness reports no cost
	Repo         string          `json:"repo"`
	BaseSHA      string          `json:"base_sha"`
	Prompt       string          `json:"prompt"`
	OutputSchema string          `json:"output_schema"` // for example "schemas/verdict.json"
	SessionID    string          `json:"session_id,omitempty"`
	Traceparent  string          `json:"traceparent,omitempty"` // W3C trace context of the task, so agent spans join its trace
	Budget       Budget          `json:"budget"`
	Instructions []string        `json:"instructions,omitempty"` // project instruction files, paths in the worktree
	Skills       []string        `json:"skills,omitempty"`       // skill ids selected for the task
	Checklist    []string        `json:"checklist,omitempty"`    // L0 checklist lines from the applicable standards
	Inputs       json.RawMessage `json:"inputs,omitempty"`
	ReadOnly     bool            `json:"read_only,omitempty"` // the role may not write to its worktree
	NoRepo       bool            `json:"no_repo,omitempty"`   // the role works in an empty directory: it must not see the repository or its instruction files

	ReviewFindingIDs []int64 `json:"review_finding_ids,omitempty"` // findings listed for recheck in the review prompt
}

// Price is dollars per million tokens.
type Price struct {
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
}

// Cost prices a token count. A nil price, or a zero cost already reported, leaves it unknown (0).
func (p *Price) Cost(in, out int) float64 {
	if p == nil {
		return 0
	}
	return (float64(in)*p.InputPerMTok + float64(out)*p.OutputPerMTok) / 1e6
}

// Budget limits one job.
type Budget struct {
	Tokens  int `json:"tokens"`
	Minutes int `json:"minutes"`
	// The supervisor's limits; nil keeps the worker's default and 0 switches a rule off.
	StallMinutes *int     `json:"stall_minutes,omitempty"`
	RepeatLimit  *int     `json:"repeat_limit,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
}

// HeartbeatRequest extends the lease.
type HeartbeatRequest struct {
	Stage string `json:"stage"` // short human-readable step, shown in the task thread
}

// EventsRequest appends progress events to the task.
type EventsRequest struct {
	Events []Event `json:"events"`
}

// Event is one progress record. The runner writes one before and after every
// agent turn and check layer.
type Event struct {
	At    time.Time       `json:"at"`
	Kind  string          `json:"kind"` // for example "turn.started", "check.finished"
	Data  json.RawMessage `json:"data,omitempty"`
	Trace string          `json:"trace_id,omitempty"`
}

// CompleteRequest finishes a job.
type CompleteRequest struct {
	ExitReason string          `json:"exit_reason"` // done | budget | rate_limit | error
	Result     json.RawMessage `json:"result,omitempty"`
	PatchPath  string          `json:"patch_path,omitempty"` // in the job's artifact directory
	Artifacts  []Artifact      `json:"artifacts"`
	Usage      Usage           `json:"usage"`
	SessionID  string          `json:"session_id,omitempty"`
	// Findings are what the runner's own checks (`nightshift check`) found after the agent
	// ran. The worker is untrusted, so each one is validated like an agent's finding.
	Findings []json.RawMessage `json:"findings,omitempty"`
	// CheckSuite identifies the local checks that ran on the patch. Partial runs start with "partial:".
	CheckSuite string `json:"check_suite,omitempty"`
}

// Artifact is a file the job wrote; the hash makes tampering visible.
type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Usage is what the job spent.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	// CacheReadTokens and CacheWriteTokens are prompt-cache input (harness.Usage says how
	// each harness counts them, and which budgets see them).
	CacheReadTokens  int     `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int     `json:"cache_write_tokens,omitempty"`
	CostUSD          float64 `json:"cost_usd"`
	Model            string  `json:"model,omitempty"` // the model the harness reported it ran
	LimitHit         bool    `json:"limit_hit"`
	// LimitUntil is when the provider says its limit resets; zero when it does not say.
	LimitUntil time.Time `json:"limit_until,omitzero"`
}

// hiddenInputs are job inputs only some roles may see: the hidden acceptance scenarios
// belong to the Verifier; Builder never gets them (design sections 7 and 16).
var hiddenInputs = map[string][]string{
	"hidden_scenarios": {"verifier"},
}

// VisibleInputs drops the inputs a role must not see. Unreadable inputs are dropped
// entirely, so a bug here fails closed.
func VisibleInputs(role string, inputs json.RawMessage) json.RawMessage {
	if len(inputs) == 0 {
		return inputs
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(inputs, &m); err != nil {
		return nil
	}
	for key, roles := range hiddenInputs {
		if !slices.Contains(roles, role) {
			delete(m, key)
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return out
}
