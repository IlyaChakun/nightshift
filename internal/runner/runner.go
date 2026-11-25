// Package runner is the worker side of the factory: it takes a job from the jobs API,
// prepares a checkout from the repository mirror, runs the role through a harness and
// returns the result, the patch and artifact hashes (design section 3).
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/stack"
)

// PatchName is the artifact that carries the job's changes.
const PatchName = "patch.diff"

// Jobs is the part of the jobs API the runner uses; jobsapi.Client implements it.
type Jobs interface {
	Claim(ctx context.Context, workerID string, wait time.Duration) (jobsapi.ClaimResponse, error)
	Heartbeat(ctx context.Context, jobID, token, stage string) error
	Events(ctx context.Context, jobID, token string, events []jobsapi.Event) error
	Complete(ctx context.Context, jobID, token string, req jobsapi.CompleteRequest) error
}

// Runner executes jobs one at a time.
type Runner struct {
	Jobs     Jobs
	WorkerID string
	// Harness returns the harness a job asks for.
	Harness func(job jobsapi.Job) (harness.Harness, error)
	// Env holds the harness credentials and the network and telemetry settings of the worker,
	// passed to the harness with every run.
	Env map[string]string
	// AgentUID and AgentGID create and own the checkout, so the agent user can write to it and
	// the runner needs no capability to change owners; zero keeps the current user. Git must be
	// configured to run as the same user.
	AgentUID, AgentGID int
	// AgentHome is the agent user's HOME; the project's verify command runs as that user
	// after the agent finished (L1, design section 8). Empty skips the check.
	AgentHome string
	// Installer installs the toolchain a project pins, in a service with its own way out (ADR 0010).
	// Without it the runner runs `mise install` itself, which needs the tool hosts in the worker's egress.
	Installer Installer
	// Schema returns the JSON Schema for a job's OutputSchema path; nil means no check.
	Schema      func(path string) ([]byte, error)
	Git         *gitops.Git
	WorkDir     string // task checkouts: <WorkDir>/<tenant>/<task>
	ArtifactDir string // job artifacts: <ArtifactDir>/<job id>
	RolesDir    string // roles/<name>.md, the instructions of each role; empty skips them
	SkillsDir   string // skills/own and skills/vendor; needed when a job selects skills
	// Stacks are the stack packs; their tools run after the project's verify command.
	Stacks    stack.Packs
	Wait      time.Duration
	Heartbeat time.Duration // zero means 30 seconds
	Log       *slog.Logger
}

func (r *Runner) log() *slog.Logger {
	if r.Log != nil {
		return r.Log
	}
	return slog.Default()
}

// Loop runs jobs until ctx ends. A failure of one job never stops the loop; a failure of
// the jobs API is retried after a pause.
func (r *Runner) Loop(ctx context.Context) error {
	for ctx.Err() == nil {
		ran, err := r.RunOne(ctx)
		if err != nil {
			r.log().ErrorContext(ctx, "runner", "error", err)
		}
		if err != nil || !ran {
			select {
			case <-ctx.Done():
			case <-time.After(pause(err)):
			}
		}
	}
	return nil
}

func pause(err error) time.Duration {
	if err != nil {
		return 5 * time.Second
	}
	return 0
}

// RunOne claims a job and runs it. It reports false when no job arrived.
func (r *Runner) RunOne(ctx context.Context) (bool, error) {
	claim, err := r.Jobs.Claim(ctx, r.WorkerID, r.Wait)
	if err != nil {
		return false, fmt.Errorf("runner: claim: %w", err)
	}
	if claim.JobID == "" {
		return false, nil
	}
	req := r.execute(ctx, claim)
	// The result must reach the book even when shutdown cancels ctx.
	done, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := r.Jobs.Complete(done, claim.JobID, claim.JobToken, req); err != nil {
		return true, fmt.Errorf("runner: complete job %s: %w", claim.JobID, err)
	}
	return true, nil
}

// execute never fails: whatever goes wrong becomes an "error" completion with a reason event.
func (r *Runner) execute(ctx context.Context, claim jobsapi.ClaimResponse) jobsapi.CompleteRequest {
	ctx, js := jobSpan(ctx, claim.JobID, claim.Job)
	defer js.End()
	req, err := r.run(ctx, claim)
	if err == nil {
		js.SetAttributes(attribute.String("nightshift.exit_reason", req.ExitReason))
		return req
	}
	js.SetStatus(codes.Error, "job failed")
	r.log().ErrorContext(ctx, "job failed", "job", claim.JobID, "role", claim.Job.Role, "error", err)
	data, _ := json.Marshal(map[string]string{"error": err.Error()}) //nolint:errchkjson // a string map always encodes
	_ = r.Jobs.Events(ctx, claim.JobID, claim.JobToken, []jobsapi.Event{{At: time.Now(), Kind: "job.error", Data: data}})
	return jobsapi.CompleteRequest{ExitReason: string(harness.ExitError), Artifacts: []jobsapi.Artifact{}}
}

func (r *Runner) run(ctx context.Context, claim jobsapi.ClaimResponse) (jobsapi.CompleteRequest, error) {
	job := claim.Job
	h, err := r.Harness(job)
	if err != nil {
		return jobsapi.CompleteRequest{}, err
	}
	var schema []byte
	if job.OutputSchema != "" && r.Schema != nil {
		if schema, err = r.Schema(job.OutputSchema); err != nil {
			return jobsapi.CompleteRequest{}, fmt.Errorf("runner: schema %s: %w", job.OutputSchema, err)
		}
	}
	workspace, err := r.prepare(ctx, job)
	if err != nil {
		return jobsapi.CompleteRequest{}, err
	}
	stop := r.beat(ctx, claim)
	defer stop()
	if err := r.prepareProject(ctx, claim, workspace); err != nil {
		return jobsapi.CompleteRequest{}, err
	}
	// The verify command and the stack tools are read before the agent runs: the agent cannot
	// choose its own checks. They come after the install, so tools that mise provides count.
	verify, tools, skipped := r.projectChecks(workspace)
	for _, name := range skipped {
		// The worker image lacks the tool; `tenant doctor` shows it, and the gap is not a failure of the patch.
		r.log().WarnContext(ctx, "stack tool skipped: the worker lacks its executable", "job", claim.JobID, "tool", name)
	}
	r.event(ctx, claim, "turn.started")
	runCtx := ctx
	if job.Budget.Minutes > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(job.Budget.Minutes)*time.Minute)
		defer cancel()
	}
	jc, err := r.buildContext(job, workspace)
	if err != nil {
		return jobsapi.CompleteRequest{}, err
	}
	defer func() { _ = os.RemoveAll(jc.Dir) }()
	req := harness.RunRequest{
		Role: harness.RoleConfig{Name: job.Role, ReadOnly: job.ReadOnly}, Workspace: workspace, Prompt: promptWithSkills(job.Prompt, jc.Skills),
		SystemFile: jc.SystemFile, OutputSchema: schema, SessionID: job.SessionID,
		Budget: harness.Budget{Tokens: job.Budget.Tokens, Minutes: job.Budget.Minutes},
	}
	if jc.SkillsDir != "" {
		req.AddDirs = []string{jc.SkillsDir}
	}
	turnCtx, turn := tracer().Start(runCtx, "agent.turn")
	req.Env = r.jobEnv(traceparent(turnCtx, job.Traceparent))
	res, err := harness.Validating{Next: h}.Run(turnCtx, req)
	turn.SetAttributes(attribute.Int("nightshift.input_tokens", res.Usage.InputTokens),
		attribute.Int("nightshift.output_tokens", res.Usage.OutputTokens))
	turn.End()
	if err != nil {
		return jobsapi.CompleteRequest{}, fmt.Errorf("runner: harness: %w", err)
	}
	r.event(ctx, claim, "turn.finished")

	// A harness that reports no cost (codex) is priced from the provider's token prices.
	if res.Usage.CostUSD == 0 {
		res.Usage.CostUSD = job.Price.Cost(res.Usage.InputTokens, res.Usage.OutputTokens)
	}
	out := jobsapi.CompleteRequest{
		ExitReason: string(res.ExitReason), Result: res.Output, SessionID: res.SessionID,
		Usage: jobsapi.Usage{
			InputTokens: res.Usage.InputTokens, OutputTokens: res.Usage.OutputTokens,
			CostUSD: res.Usage.CostUSD, LimitHit: res.Usage.LimitHit,
		},
		Artifacts: []jobsapi.Artifact{},
	}
	patch, err := r.Git.Diff(ctx, workspace, job.BaseSHA)
	if err != nil {
		return jobsapi.CompleteRequest{}, err
	}
	if len(patch) > 0 {
		art, err := r.writeArtifact(claim.JobID, PatchName, patch)
		if err != nil {
			return jobsapi.CompleteRequest{}, err
		}
		out.PatchPath, out.Artifacts = PatchName, append(out.Artifacts, art)
		checkCtx, checks := tracer().Start(ctx, "checks")
		out.Findings, err = r.runChecks(checkCtx, claim, workspace, job.BaseSHA, verify, tools)
		checks.End()
		if err != nil {
			return jobsapi.CompleteRequest{}, err
		}
	}
	return out, nil
}

// jobEnv is the harness environment of one job: the credentials, and TRACEPARENT so spans
// the agent emits join the task's trace.
func (r *Runner) jobEnv(tp string) map[string]string {
	env := make(map[string]string, len(r.Env)+1)
	maps.Copy(env, r.Env)
	r.withShims(env)
	if tp != "" {
		env["TRACEPARENT"] = tp
	}
	return env
}

// prepare makes a fresh checkout of the job's base commit for the task.
func (r *Runner) prepare(ctx context.Context, job jobsapi.Job) (string, error) {
	mirror, err := r.Git.MirrorPath(job.Repo)
	if err != nil {
		return "", err
	}
	if !safeSegment(job.Tenant) || !safeSegment(job.TaskID) {
		return "", fmt.Errorf("runner: unsafe tenant or task id %q/%q", job.Tenant, job.TaskID)
	}
	dest := filepath.Join(r.WorkDir, job.Tenant, job.TaskID)
	if err := r.freshDir(ctx, dest); err != nil {
		return "", err
	}
	if err := r.Git.Checkout(ctx, mirror, dest, job.BaseSHA); err != nil {
		return "", err
	}
	return dest, nil
}

// freshDir removes an earlier checkout at dest and creates its parent, both as the agent user,
// who owns the work directory.
func (r *Runner) freshDir(ctx context.Context, dest string) error {
	for _, args := range [][]string{{"rm", "-rf", "--", dest}, {"mkdir", "-p", "--", filepath.Dir(dest)}} {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // G204: fixed commands on a path built from validated segments
		if r.AgentUID != 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(r.AgentUID), Gid: uint32(r.AgentGID)}} //nolint:gosec // ids come from the worker's own configuration
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("runner: %s %s: %w: %s", args[0], dest, err, out)
		}
	}
	return nil
}

func safeSegment(s string) bool {
	return s != "" && s != "." && s != ".." && filepath.Base(s) == s
}

func (r *Runner) writeArtifact(jobID, name string, data []byte) (jobsapi.Artifact, error) {
	if !safeSegment(jobID) {
		return jobsapi.Artifact{}, errors.New("runner: unsafe job id")
	}
	dir := filepath.Join(r.ArtifactDir, jobID)
	if err := os.MkdirAll(dir, 0o751); err != nil { //nolint:gosec // nightshift reads through the group, the agent only passes through to its check directory
		return jobsapi.Artifact{}, fmt.Errorf("runner: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o640); err != nil { //nolint:gosec // the group is nightshift, which applies the patch
		return jobsapi.Artifact{}, fmt.Errorf("runner: write artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return jobsapi.Artifact{Path: name, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}, nil
}

func (r *Runner) event(ctx context.Context, claim jobsapi.ClaimResponse, kind string) {
	err := r.Jobs.Events(ctx, claim.JobID, claim.JobToken, []jobsapi.Event{{At: time.Now(), Kind: kind}})
	if err != nil {
		r.log().WarnContext(ctx, "event not recorded", "job", claim.JobID, "kind", kind, "error", err)
	}
}

// beat extends the lease while the job runs. A heartbeat that fails is logged; the lease
// then runs out and the book requeues the job.
func (r *Runner) beat(ctx context.Context, claim jobsapi.ClaimResponse) (stop func()) {
	every := r.Heartbeat
	if every <= 0 {
		every = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := r.Jobs.Heartbeat(ctx, claim.JobID, claim.JobToken, ""); err != nil && ctx.Err() == nil {
					r.log().WarnContext(ctx, "heartbeat failed", "job", claim.JobID, "error", err)
				}
			}
		}
	}()
	return func() { cancel(); <-done }
}

// Installer installs the toolchain pinned in dir before the agent starts.
type Installer interface {
	Install(ctx context.Context, job, dir string) (output string, err error)
}
