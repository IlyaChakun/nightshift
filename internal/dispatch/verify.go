package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/policy"
)

// verifying runs the Verifier leg of the review state. It reports false when the tenant has
// no Verifier (no role, or no provider in its chain): the gate then passes the state without
// a verdict, as it did before the stage existed. With a Verifier it enqueues a read-only job
// on the pushed head, waits for it and acts on its verdict.
func (c *Conductor) verifying(ctx context.Context, t config.Tenant, task bookdb.BookTask) (handled bool, err error) {
	role, ok := c.Roles["verifier"]
	if !ok || len(c.Router.Chains["verifier"]) == 0 {
		return false, nil
	}
	jobs, err := bookdb.New(c.Pool).ListTaskJobs(ctx, bookdb.ListTaskJobsParams{TaskID: task.ID, Role: "verifier"})
	if err != nil {
		return true, fmt.Errorf("conductor: verifier jobs of task %d: %w", task.ID, err)
	}
	if len(jobs) == 0 || jobs[len(jobs)-1].ConsumedAt.Valid {
		return true, c.enqueueVerifier(ctx, task, role)
	}
	last := jobs[len(jobs)-1]
	switch last.State {
	case "queued", "running":
		return true, nil
	case "done":
		return true, c.consumeVerdict(ctx, t, task, last, role.Limits.MaxReturns, len(jobs)-1)
	default:
		return true, c.escalate(ctx, t, task, last, "the Verifier job ended with "+last.ExitReason.String)
	}
}

// enqueueVerifier queues a review job. The worker checks out the pushed head; the base the
// change is measured against is an input, so the Verifier reads `git diff <base>...HEAD`.
func (c *Conductor) enqueueVerifier(ctx context.Context, task bookdb.BookTask, role config.Role) error {
	choice, err := c.Router.Choose(ctx, "verifier")
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if choice.Decision != Run || !task.HeadSha.Valid {
		return nil // every provider is busy, or the head is not known yet; the next tick tries again
	}
	prov := c.Router.Providers[choice.Provider]
	mirror, err := c.mirror(ctx, task.Repo)
	if err != nil {
		return err
	}
	base, err := c.Git.Rev(ctx, mirror, c.base())
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	instructions, checklist, err := c.projectContext(ctx, mirror, base)
	if err != nil {
		return err
	}
	inputs, err := json.Marshal(map[string]string{"mode": "review", "base_sha": base, "head_sha": task.HeadSha.String})
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	job := jobsapi.Job{
		Traceparent:  Traceparent(taskContext(ctx, task)),
		Instructions: instructions, Checklist: checklist, ReadOnly: role.ReadOnly(), Inputs: inputs,
		Harness: prov.Harness, Auth: prov.Auth, Model: prov.Model, BaseURL: prov.BaseURL, KeyEnv: prov.KeyEnv,
		Repo: task.Repo, BaseSHA: task.HeadSha.String, OutputSchema: role.OutputSchema, Skills: role.Skills,
		Prompt: "Review mode. Task: " + task.Title + "\nThe change is `git diff " + base + "...HEAD` in the checked-out worktree.",
		Budget: jobsapi.Budget{Tokens: role.Limits.Tokens, Minutes: role.Limits.Minutes},
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if _, err := bookdb.New(c.Pool).EnqueueJob(ctx, bookdb.EnqueueJobParams{
		Tenant: task.Tenant, TaskID: task.ID, Role: "verifier", Provider: choice.Provider, Payload: payload,
	}); err != nil {
		return fmt.Errorf("conductor: enqueue: %w", err)
	}
	return nil
}

// consumeVerdict stores the findings of a finished review and applies the verdict. The job is
// consumed before the move: a crash between the two repeats the move, which a lost race
// (flow.ErrConflict) turns into a no-op, and never reads the same verdict twice.
func (c *Conductor) consumeVerdict(ctx context.Context, t config.Tenant, task bookdb.BookTask, job bookdb.BookJob, maxReturns, returned int) error {
	out, err := Decide(job.Result, policy.Low, returned, maxReturns)
	if err != nil {
		return c.escalate(ctx, t, task, job, err.Error())
	}
	var res struct {
		Findings []struct {
			RuleID, Category, Severity, LayerCaught, Message string
			Location                                         struct{ Path string }
		} `json:"findings"`
	}
	// Decide read the same bytes, so this cannot fail on syntax; a shape mismatch only loses findings.
	_ = json.Unmarshal(job.Result, &res)
	q := bookdb.New(c.Pool)
	if out.Event == flow.Returned {
		for _, f := range res.Findings {
			sum := sha256.Sum256([]byte(f.RuleID + "\x00" + f.Location.Path + "\x00" + f.Message))
			if _, err := q.AddFinding(ctx, bookdb.AddFindingParams{
				Tenant: task.Tenant, TaskID: task.ID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
				RuleID: f.RuleID, Category: f.Category, Severity: f.Severity, LayerCaught: f.LayerCaught,
				Message: f.Message, Fingerprint: "verifier:" + hex.EncodeToString(sum[:8]),
			}); err != nil {
				return fmt.Errorf("conductor: %w", err)
			}
		}
	}
	if out.Raised != policy.Low {
		c.appendEvent(ctx, task, eventRaised, string(out.Raised))
	}
	if _, err := q.ConsumeJob(ctx, job.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("conductor: %w", err)
	}
	if out.Why != "" {
		c.log().WarnContext(ctx, "task escalated", "task", task.ID, "why", out.Why)
	}
	_, err = c.move(ctx, t, task, out.Event, "")
	return err
}
