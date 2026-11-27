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
	"regexp"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/ilyachakun/nightshift/internal/assurance/check"
	"github.com/ilyachakun/nightshift/internal/customization/stack"
	"github.com/ilyachakun/nightshift/internal/execution/gitops"
	"github.com/ilyachakun/nightshift/internal/execution/harness"
	"github.com/ilyachakun/nightshift/internal/execution/harness/supervise"
	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

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
	// Supervise holds the limits every run is watched against: silence, a repeated tool call and
	// money. The tokens of the job's budget apply unless the policy sets its own.
	Supervise supervise.Policy
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
	WorkDir     string // checkouts, one per attempt: <WorkDir>/<tenant>/<task>-<attempt>
	ArtifactDir string // job artifacts, one directory per attempt: <ArtifactDir>/<job id>/<attempt>
	RolesDir    string // roles/<name>.md, the instructions of each role; empty skips them
	SkillsDir   string // skills/own and skills/vendor; needed when a job selects skills
	// StandardsDir holds the standards pages the job's checklist points to; the agent gets it
	// as an extra directory when it exists (jobsapi.StandardsDir in the image).
	StandardsDir string
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
	r.sweep(ctx)
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

// sweep removes the checkouts a crashed run left behind. Each attempt has its own directory,
// so no later attempt removes them, and none may outlive its job (design section 17). It runs
// before the first claim; another worker replica on the same volume would lose its live
// checkouts and its jobs would be retried (plan M3).
func (r *Runner) sweep(ctx context.Context) {
	tenants, err := os.ReadDir(r.WorkDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			r.log().WarnContext(ctx, "work directory not swept", "error", err)
		}
		return
	}
	for _, t := range tenants {
		dir := filepath.Join(r.WorkDir, t.Name())
		if !t.IsDir() || !safeSegment(t.Name()) {
			continue
		}
		left, err := os.ReadDir(dir)
		if err != nil {
			r.log().WarnContext(ctx, "work directory not swept", "dir", dir, "error", err)
			continue
		}
		for _, e := range left {
			if !attemptDir.MatchString(e.Name()) {
				continue // not a checkout: another service's files on the same volume stay
			}
			if err := r.removeAsAgent(ctx, filepath.Join(dir, e.Name())); err != nil {
				r.log().WarnContext(ctx, "leftover checkout not removed", "dir", dir, "error", err)
			}
		}
	}
}

func pause(err error) time.Duration {
	if err != nil {
		return 5 * time.Second
	}
	return 0
}

// DeliverTimeout bounds the calls that must reach the book after shutdown ended the job: the
// result and the reason of a failure.
const DeliverTimeout = 30 * time.Second

// RunOne claims a job and runs it. It reports false when no job arrived.
func (r *Runner) RunOne(ctx context.Context) (bool, error) {
	claim, err := r.Jobs.Claim(ctx, r.WorkerID, r.Wait)
	if err != nil {
		return false, fmt.Errorf("runner: claim: %w", err)
	}
	if claim.JobID == "" {
		return false, nil
	}
	req, reason := r.execute(ctx, claim)
	// The result and the reason of a failure must reach the book even when shutdown cancels ctx;
	// they share one deadline, so the reason cannot use up the time the result needs.
	done, cancel := context.WithTimeout(context.WithoutCancel(ctx), DeliverTimeout)
	defer cancel()
	if reason != nil {
		if err := r.Jobs.Events(done, claim.JobID, claim.JobToken, []jobsapi.Event{*reason}); err != nil {
			r.log().WarnContext(ctx, "error reason not recorded", "job", claim.JobID, "error", err)
		}
	}
	if err := r.Jobs.Complete(done, claim.JobID, claim.JobToken, req); err != nil {
		return true, fmt.Errorf("runner: complete job %s: %w", claim.JobID, err)
	}
	return true, nil
}

// execute never fails: whatever goes wrong becomes an "error" completion and its reason event,
// which RunOne delivers with the result.
func (r *Runner) execute(ctx context.Context, claim jobsapi.ClaimResponse) (jobsapi.CompleteRequest, *jobsapi.Event) {
	ctx, js := jobSpan(ctx, claim.JobID, claim.Job)
	defer js.End()
	req, err := r.run(ctx, claim)
	if err == nil {
		js.SetAttributes(attribute.String("nightshift.exit_reason", req.ExitReason))
		return req, nil
	}
	js.SetStatus(codes.Error, "job failed")
	r.log().ErrorContext(ctx, "job failed", "job", claim.JobID, "role", claim.Job.Role, "error", err)
	data, _ := json.Marshal(map[string]string{"error": err.Error()}) //nolint:errchkjson // a string map always encodes
	return jobsapi.CompleteRequest{ExitReason: string(harness.ExitError), Artifacts: []jobsapi.Artifact{}},
		&jobsapi.Event{At: time.Now(), Kind: "job.error", Data: data}
}

func (r *Runner) run(ctx context.Context, claim jobsapi.ClaimResponse) (jobsapi.CompleteRequest, error) {
	claimed := time.Now()
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
	workspace, err := r.prepare(ctx, job, attemptOf(claim))
	if err != nil {
		return jobsapi.CompleteRequest{}, err
	}
	// No checkout outlives its job: all jobs run as one agent user, so one left behind would be
	// readable by the next job, of any tenant (design section 17, "Worker state"). The next job
	// of the task checks out afresh anyway.
	defer r.dropCheckout(ctx, claim.JobID, workspace)
	ctx, stop := r.beat(ctx, claim, claimed)
	defer stop()
	if err := r.prepareProject(ctx, claim, workspace); err != nil {
		return jobsapi.CompleteRequest{}, err
	}
	// The verify command and the stack tools are read before the agent runs: the agent cannot
	// choose its own checks. They come after the install, so tools that mise provides count.
	home := r.homeFor(job.Tenant)
	verify, tools, skipped := r.projectChecks(home, workspace)
	for _, name := range skipped {
		// The worker image lacks the tool; `tenant doctor` shows it, and the gap is not a failure of the patch.
		r.log().WarnContext(ctx, "stack tool skipped: the worker lacks its executable", "job", claim.JobID, "tool", name)
	}
	if err := r.startTurn(ctx, claim); err != nil {
		return jobsapi.CompleteRequest{}, err
	}
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
		SystemFile: jc.SystemFile, ProjectInstructions: len(job.Instructions) > 0, OutputSchema: schema, SessionID: job.SessionID,
		Budget: harness.Budget{Tokens: job.Budget.Tokens, Minutes: job.Budget.Minutes},
	}
	if jc.SkillsDir != "" {
		req.AddDirs = []string{jc.SkillsDir}
	}
	if r.StandardsDir != "" {
		if info, err := os.Stat(r.StandardsDir); err == nil && info.IsDir() {
			req.AddDirs = append(req.AddDirs, r.StandardsDir)
		}
	}
	turnCtx, turn := tracer().Start(runCtx, "agent.turn")
	req.Env = r.jobEnv(traceparent(turnCtx, job.Traceparent), home)
	res, err := harness.Validating{Next: supervise.Supervisor{Next: h, Policy: r.policy(job)}}.Run(turnCtx, req)
	turn.SetAttributes(attribute.Int("nightshift.input_tokens", res.Usage.InputTokens),
		attribute.Int("nightshift.output_tokens", res.Usage.OutputTokens),
		attribute.Int("nightshift.cache_read_tokens", res.Usage.CacheReadTokens),
		attribute.Int("nightshift.cache_write_tokens", res.Usage.CacheWriteTokens))
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
			CacheReadTokens: res.Usage.CacheReadTokens, CacheWriteTokens: res.Usage.CacheWriteTokens,
			CostUSD: res.Usage.CostUSD, Model: res.Usage.Model, LimitHit: res.Usage.LimitHit, LimitUntil: res.Usage.LimitUntil,
		},
		Artifacts: []jobsapi.Artifact{},
	}
	if job.NoRepo {
		return out, nil // no checkout, so no patch to take
	}
	return r.withPatch(ctx, claim, workspace, verify, tools, out)
}

// withPatch adds the attempt's patch to the result and runs the checks on it; a job that
// changed nothing has no patch to check.
func (r *Runner) withPatch(ctx context.Context, claim jobsapi.ClaimResponse, workspace, verify string, tools []check.Tool, out jobsapi.CompleteRequest) (jobsapi.CompleteRequest, error) {
	base := claim.Job.BaseSHA
	patch, err := r.Git.Diff(ctx, workspace, base)
	if err != nil || len(patch) == 0 {
		return out, err
	}
	art, err := r.writeArtifact(claim.JobID, attemptOf(claim), jobsapi.PatchName, patch)
	if err != nil {
		return jobsapi.CompleteRequest{}, err
	}
	out.PatchPath, out.Artifacts = art.Path, append(out.Artifacts, art)
	checkCtx, checks := tracer().Start(ctx, "checks")
	out.Findings, out.CheckSuite, err = r.runChecks(checkCtx, claim, workspace, base, verify, tools, patch)
	checks.End()
	if err != nil {
		return jobsapi.CompleteRequest{}, err
	}
	return out, nil
}

// startTurn records the start of the agent's turn; a turn never starts on a lease this
// attempt no longer holds.
func (r *Runner) startTurn(ctx context.Context, claim jobsapi.ClaimResponse) error {
	if ctx.Err() != nil {
		return fmt.Errorf("runner: turn not started: %w", context.Cause(ctx))
	}
	r.event(ctx, claim, "turn.started")
	return nil
}

// policy is the supervisor policy of one job: the worker's, then the job's own limits on top.
// The job's token budget applies when the policy sets no tokens of its own.
func (r *Runner) policy(job jobsapi.Job) supervise.Policy {
	p := r.Supervise
	if p.Tokens == 0 {
		p.Tokens = job.Budget.Tokens
	}
	if v := job.Budget.StallMinutes; v != nil {
		p.StallAfter = time.Duration(*v) * time.Minute
	}
	if v := job.Budget.RepeatLimit; v != nil {
		p.RepeatLimit = *v
	}
	if v := job.Budget.CostUSD; v != nil {
		p.CostUSD = *v
	}
	if p.Price == nil {
		p.Price = job.Price.Cost
	}
	return p
}

// jobEnv is the harness environment of one job: the credentials, and TRACEPARENT so spans
// the agent emits join the task's trace.
func (r *Runner) jobEnv(tp, home string) map[string]string {
	env := make(map[string]string, len(r.Env)+1)
	maps.Copy(env, r.Env)
	r.withShims(env, home)
	if tp != "" {
		env["TRACEPARENT"] = tp
	}
	return env
}

// attemptOf names one lease of a job: the job id stays the same when the book hands the job
// to another attempt, the token does not. The name is a hash, so it does not reveal the token.
func attemptOf(claim jobsapi.ClaimResponse) string {
	sum := sha256.Sum256([]byte(claim.JobID + "\x00" + claim.JobToken))
	return hex.EncodeToString(sum[:8])
}

// prepare makes a fresh checkout of the job's base commit in a directory of its own attempt,
// so an attempt that lost its lease never shares or removes the checkout of the next one.
func (r *Runner) prepare(ctx context.Context, job jobsapi.Job, attempt string) (string, error) {
	var mirror string
	if !job.NoRepo {
		var err error
		if mirror, err = r.Git.MirrorPath(job.Repo); err != nil {
			return "", err
		}
	}
	if !safeSegment(job.Tenant) || !safeSegment(job.TaskID) {
		return "", fmt.Errorf("runner: unsafe tenant or task id %q/%q", job.Tenant, job.TaskID)
	}
	if err := r.realDir(ctx, filepath.Join(r.WorkDir, job.Tenant)); err != nil {
		return "", err
	}
	dest := filepath.Join(r.WorkDir, job.Tenant, job.TaskID+"-"+attempt)
	if err := r.freshDir(ctx, dest); err != nil {
		return "", err
	}
	if job.NoRepo {
		// An independent reader sees the prompt and nothing else: no checkout, so no AGENTS.md either.
		return dest, r.asAgent(ctx, "mkdir", "--", dest)
	}
	if err := r.Git.Checkout(ctx, mirror, dest, job.BaseSHA); err != nil {
		return "", err
	}
	return dest, nil
}

// freshDir removes an earlier checkout at dest and creates its parent, both as the agent user,
// who owns the work directory.
func (r *Runner) freshDir(ctx context.Context, dest string) error {
	if err := r.removeAsAgent(ctx, dest); err != nil {
		return err
	}
	return r.asAgent(ctx, "mkdir", "-p", "--", filepath.Dir(dest))
}

// dropCheckout removes a job's checkout once the job ends, also when it was canceled.
func (r *Runner) dropCheckout(ctx context.Context, jobID, workspace string) {
	if err := r.removeAsAgent(context.WithoutCancel(ctx), workspace); err != nil {
		r.log().WarnContext(ctx, "checkout not removed", "job", jobID, "error", err)
	}
}

// removeAsAgent deletes a tree the agent owns, first giving the agent write access to every
// directory in it, so a directory the agent made read-only cannot block the next job.
func (r *Runner) removeAsAgent(ctx context.Context, dir string) error {
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	chmodErr := r.asAgent(ctx, "chmod", "-R", "u+rwX", "--", dir)
	if err := r.asAgent(ctx, "rm", "-rf", "--", dir); err != nil {
		return errors.Join(chmodErr, err)
	}
	return nil
}

// realDir makes sure a tenant's directory in the work root is a directory and not a link that
// an earlier agent planted to send the checkout somewhere nobody cleans.
func (r *Runner) realDir(ctx context.Context, dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) || (err == nil && info.IsDir()) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("runner: %w", err)
	}
	return r.asAgent(ctx, "rm", "-f", "--", dir)
}

// asAgent runs a fixed file command as the agent user, who owns the work directory.
func (r *Runner) asAgent(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: fixed commands on a path built from validated segments
	if r.AgentUID != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(r.AgentUID), Gid: uint32(r.AgentGID)}} //nolint:gosec // ids come from the worker's own configuration
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("runner: %s: %w: %s", strings.Join(cmd.Args, " "), err, out)
	}
	return nil
}

func safeSegment(s string) bool {
	return s != "" && s != "." && s != ".." && filepath.Base(s) == s
}

// writeArtifact stores an artifact under the job and its attempt; the recorded path names the
// attempt, so the core reads the file of the attempt that completed the job.
func (r *Runner) writeArtifact(jobID, attempt, name string, data []byte) (jobsapi.Artifact, error) {
	if !safeSegment(jobID) {
		return jobsapi.Artifact{}, errors.New("runner: unsafe job id")
	}
	dir := filepath.Join(r.ArtifactDir, jobID, attempt)
	if err := os.MkdirAll(dir, 0o751); err != nil { //nolint:gosec // nightshift reads through the group, the agent only passes through to its check directory
		return jobsapi.Artifact{}, fmt.Errorf("runner: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o640); err != nil { //nolint:gosec // the group is nightshift, which applies the patch
		return jobsapi.Artifact{}, fmt.Errorf("runner: write artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return jobsapi.Artifact{Path: attempt + "/" + name, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}, nil
}

func (r *Runner) event(ctx context.Context, claim jobsapi.ClaimResponse, kind string) {
	err := r.Jobs.Events(ctx, claim.JobID, claim.JobToken, []jobsapi.Event{{At: time.Now(), Kind: kind}})
	if err != nil {
		r.log().WarnContext(ctx, "event not recorded", "job", claim.JobID, "kind", kind, "error", err)
	}
}

// attemptDir is the name of one attempt's checkout in a tenant directory, as prepare makes it.
var attemptDir = regexp.MustCompile(`^.+-[0-9a-f]{16}$`)

// errLeaseLost ends a job whose lease the jobs API no longer holds for this attempt.
var errLeaseLost = errors.New("runner: lease lost")

// beat extends the lease while the job runs. The returned context ends with errLeaseLost
// when the jobs API refuses the token, or when no heartbeat succeeded for as long as a lease
// lasts: the book may then have handed the job to another attempt, and this one must not
// change what that attempt reads or delivers. A claim without an expiry keeps no deadline.
//
// claimed is when the claim arrived: the lease ran from then, and every successful heartbeat
// extends it by the same length.
func (r *Runner) beat(ctx context.Context, claim jobsapi.ClaimResponse, claimed time.Time) (context.Context, func()) {
	every := r.Heartbeat
	if every <= 0 {
		every = 30 * time.Second
	}
	ctx, cancel := context.WithCancelCause(ctx)
	var lease time.Duration
	if !claim.ExpiresAt.IsZero() {
		if lease = claim.ExpiresAt.Sub(claimed); lease <= 0 || time.Since(claimed) >= lease {
			cancel(errLeaseLost)
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(every)
		defer t.Stop()
		lastOK := claimed
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			sent := time.Now()
			// A heartbeat that hangs must not hold the deadline back.
			hb, stop := context.WithTimeout(ctx, every)
			err := r.Jobs.Heartbeat(hb, claim.JobID, claim.JobToken, "")
			stop()
			switch {
			case err == nil:
				lastOK = sent
			case ctx.Err() != nil:
				return
			case errors.Is(err, jobsapi.ErrLeaseLost):
				r.log().WarnContext(ctx, "lease lost; stopping the job", "job", claim.JobID, "error", err)
				cancel(errLeaseLost)
				return
			default:
				r.log().WarnContext(ctx, "heartbeat failed", "job", claim.JobID, "error", err)
			}
			if lease > 0 && time.Since(lastOK) >= lease {
				r.log().WarnContext(ctx, "lease ran out without a heartbeat; stopping the job", "job", claim.JobID)
				cancel(errLeaseLost)
				return
			}
		}
	}()
	return ctx, func() { cancel(nil); <-done }
}

// Installer installs the toolchain pinned in dir before the agent starts.
type Installer interface {
	Install(ctx context.Context, job, dir string) (output string, err error)
}
