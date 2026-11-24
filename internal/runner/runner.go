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
	"os"
	"path/filepath"
	"time"

	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
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
	// Harness returns the harness for a provider name.
	Harness func(provider string) (harness.Harness, error)
	// Schema returns the JSON Schema for a job's OutputSchema path; nil means no check.
	Schema      func(path string) ([]byte, error)
	Git         *gitops.Git
	WorkDir     string // task checkouts: <WorkDir>/<tenant>/<task>
	ArtifactDir string // job artifacts: <ArtifactDir>/<job id>
	Wait        time.Duration
	Heartbeat   time.Duration // zero means 30 seconds
	Log         *slog.Logger
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
	req, err := r.run(ctx, claim)
	if err == nil {
		return req
	}
	r.log().ErrorContext(ctx, "job failed", "job", claim.JobID, "role", claim.Job.Role, "error", err)
	data, _ := json.Marshal(map[string]string{"error": err.Error()}) //nolint:errchkjson // a string map always encodes
	_ = r.Jobs.Events(ctx, claim.JobID, claim.JobToken, []jobsapi.Event{{At: time.Now(), Kind: "job.error", Data: data}})
	return jobsapi.CompleteRequest{ExitReason: string(harness.ExitError), Artifacts: []jobsapi.Artifact{}}
}

func (r *Runner) run(ctx context.Context, claim jobsapi.ClaimResponse) (jobsapi.CompleteRequest, error) {
	job := claim.Job
	h, err := r.Harness(job.Provider)
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
	r.event(ctx, claim, "turn.started")
	runCtx := ctx
	if job.Budget.Minutes > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(job.Budget.Minutes)*time.Minute)
		defer cancel()
	}
	res, err := harness.Validating{Next: h}.Run(runCtx, harness.RunRequest{
		Role: harness.RoleConfig{Name: job.Role}, Workspace: workspace, Prompt: job.Prompt,
		OutputSchema: schema, SessionID: job.SessionID,
		Budget: harness.Budget{Tokens: job.Budget.Tokens, Minutes: job.Budget.Minutes},
	})
	if err != nil {
		return jobsapi.CompleteRequest{}, fmt.Errorf("runner: harness: %w", err)
	}
	r.event(ctx, claim, "turn.finished")

	req := jobsapi.CompleteRequest{
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
		req.PatchPath, req.Artifacts = PatchName, append(req.Artifacts, art)
	}
	return req, nil
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
	if err := os.RemoveAll(dest); err != nil {
		return "", fmt.Errorf("runner: clear %s: %w", dest, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return "", fmt.Errorf("runner: %w", err)
	}
	if err := r.Git.Checkout(ctx, mirror, dest, job.BaseSHA); err != nil {
		return "", err
	}
	return dest, nil
}

func safeSegment(s string) bool {
	return s != "" && s != "." && s != ".." && filepath.Base(s) == s
}

func (r *Runner) writeArtifact(jobID, name string, data []byte) (jobsapi.Artifact, error) {
	if !safeSegment(jobID) {
		return jobsapi.Artifact{}, errors.New("runner: unsafe job id")
	}
	dir := filepath.Join(r.ArtifactDir, jobID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return jobsapi.Artifact{}, fmt.Errorf("runner: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
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
