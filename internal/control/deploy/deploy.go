// Package deploy runs the deploy steps of a task through a journal (design section 17,
// "Deploy targets", ADR 0014): bring a preview up and smoke it, or release a merged revision,
// smoke production and roll back on its own when the smoke fails. Each step is written to the
// journal before it runs and again when it is done, so after a restart the steps continue from
// the journal: a release is never repeated, and a rollback still knows the revision that was
// live before. The platform is behind deployapi.Deployer; this package knows none.
package deploy

import (
	"context"
	"errors"
	"fmt"

	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
)

// Journal steps.
const (
	StepUp           = "up"
	StepSmoke        = "smoke"
	StepRelease      = "release"
	StepReleaseSmoke = "release_smoke"
	StepRollback     = "rollback"
)

// Step is one journal row.
type Step struct {
	Name     string
	Done     bool
	Revision deployapi.Revision
	Previous deployapi.Revision
	OK       bool
	Detail   string
}

// Journal stores the deploy steps of tasks.
type Journal interface {
	// Start records a step before it runs. A step that is already there is returned as it is.
	Start(ctx context.Context, tenant string, taskID int64, step string) (Step, error)
	Finish(ctx context.Context, taskID int64, s Step) error
	// ClearPreview forgets the preview steps, so a changed head gets a new preview.
	ClearPreview(ctx context.Context, taskID int64) error
	// Reset forgets one step, so it runs again.
	Reset(ctx context.Context, taskID int64, step string) error
}

// start records a step, and forgets a finished one that belongs to another revision: a task
// that came back from a rollback releases a new revision and must not reuse the old answers.
func (r *Runner) start(ctx context.Context, job Job, step string) (Step, error) {
	st, err := r.Journal.Start(ctx, job.Tenant, job.TaskID, step)
	if err != nil {
		return Step{}, fmt.Errorf("deploy: %w", err)
	}
	if !st.Done || st.Revision == job.Rev {
		return st, nil
	}
	if err := r.Journal.Reset(ctx, job.TaskID, step); err != nil {
		return Step{}, fmt.Errorf("deploy: %w", err)
	}
	if st, err = r.Journal.Start(ctx, job.Tenant, job.TaskID, step); err != nil {
		return Step{}, fmt.Errorf("deploy: %w", err)
	}
	return st, nil
}

// Runner runs deploy steps for tasks.
type Runner struct {
	Journal  Journal
	Deployer deployapi.Deployer
}

// Job is one task's deploy work.
type Job struct {
	Tenant string
	TaskID int64
	// TaskKey names the preview environment (the task's id in text); empty for production.
	TaskKey string
	// Rev is the revision under test (the head of the task branch) or the one to release (the merge commit).
	Rev deployapi.Revision
}

// PreviewOutcome says how a preview run ended.
type PreviewOutcome string

// Outcomes of Preview.
const (
	// PreviewReady: the preview is up and its smoke passed; Endpoint says where it answers.
	PreviewReady PreviewOutcome = "ready"
	// PreviewBroken: the preview came up but is unhealthy or failed its smoke: a defect in the
	// code under test, which goes back to Builder with Detail.
	PreviewBroken PreviewOutcome = "broken"
)

// PreviewResult is what Preview reports.
type PreviewResult struct {
	Outcome  PreviewOutcome
	Endpoint deployapi.Endpoint
	Detail   string
}

// Preview brings the task's preview up at job.Rev and smokes it. An error from the platform is
// returned as it is: the environment may be fine, so the caller retries or escalates and does
// not blame the code. A smoke that fails is a PreviewBroken outcome.
func (r *Runner) Preview(ctx context.Context, job Job) (PreviewResult, error) {
	env := deployapi.EnvRef{Tenant: job.Tenant, Task: job.TaskKey}
	up, err := r.start(ctx, job, StepUp)
	if err != nil {
		return PreviewResult{}, err
	}
	// Up is idempotent, so it runs again after a restart and returns the endpoint.
	ep, err := r.Deployer.Up(ctx, env, job.Rev)
	if errors.Is(err, deployapi.ErrRefused) {
		// The change breaks a rule of the recipe: a defect of the code under test, not of the platform.
		return PreviewResult{Outcome: PreviewBroken, Detail: err.Error()}, nil
	}
	if err != nil {
		return PreviewResult{}, fmt.Errorf("deploy: up: %w", err)
	}
	if !up.Done {
		if err := r.Journal.Finish(ctx, job.TaskID, Step{Name: StepUp, Revision: job.Rev, OK: true}); err != nil {
			return PreviewResult{}, fmt.Errorf("deploy: %w", err)
		}
	}
	sm, err := r.smoke(ctx, job, env, StepSmoke)
	if err != nil {
		return PreviewResult{}, err
	}
	if !sm.OK {
		return PreviewResult{Outcome: PreviewBroken, Endpoint: ep, Detail: sm.Detail}, nil
	}
	return PreviewResult{Outcome: PreviewReady, Endpoint: ep}, nil
}

// Teardown removes the task's preview and forgets its steps. Down of an absent preview is fine.
func (r *Runner) Teardown(ctx context.Context, job Job) error {
	if err := r.Deployer.Down(ctx, deployapi.EnvRef{Tenant: job.Tenant, Task: job.TaskKey}); err != nil {
		return fmt.Errorf("deploy: down: %w", err)
	}
	if err := r.Journal.ClearPreview(ctx, job.TaskID); err != nil {
		return fmt.Errorf("deploy: %w", err)
	}
	return nil
}

// ReleaseOutcome says how a release run ended.
type ReleaseOutcome string

// Outcomes of Release.
const (
	// Released: the revision is live and its smoke passed.
	Released ReleaseOutcome = "released"
	// RolledBack: the smoke failed and the previous revision is live again.
	RolledBack ReleaseOutcome = "rolled_back"
	// RollbackImpossible: the smoke failed and there is no previous revision to return to;
	// a person must act.
	RollbackImpossible ReleaseOutcome = "rollback_impossible"
)

// ReleaseResult is what Release reports.
type ReleaseResult struct {
	Outcome  ReleaseOutcome
	Previous deployapi.Revision
	Detail   string
}

// Release makes job.Rev live, smokes production and rolls back on its own when the smoke fails.
// An error means a step could not finish; call Release again and it continues from the journal.
func (r *Runner) Release(ctx context.Context, job Job) (ReleaseResult, error) {
	rel, err := r.start(ctx, job, StepRelease)
	if err != nil {
		return ReleaseResult{}, err
	}
	prev := rel.Previous
	var sm deployapi.SmokeResult
	failed := rel.Done && !rel.OK // a rollout that did not come up, recorded before a restart
	if failed {
		sm = deployapi.SmokeResult{Detail: rel.Detail}
	}
	if !rel.Done {
		got, err := r.Deployer.Release(ctx, job.Rev)
		switch {
		case errors.Is(err, deployapi.ErrRolloutFailed):
			// The new revision started and is not healthy: no smoke can pass, so the way back
			// is the same as after a failed smoke.
			failed, prev, sm = true, got.Previous, deployapi.SmokeResult{Detail: err.Error()}
		case err != nil:
			return ReleaseResult{}, fmt.Errorf("deploy: release: %w", err)
		default:
			prev = got.Previous
		}
		if err := r.Journal.Finish(ctx, job.TaskID, Step{Name: StepRelease, Revision: job.Rev, Previous: prev, OK: !failed, Detail: sm.Detail}); err != nil {
			return ReleaseResult{}, fmt.Errorf("deploy: %w", err)
		}
	}
	if !failed {
		var err error
		if sm, err = r.smoke(ctx, job, deployapi.EnvRef{Tenant: job.Tenant}, StepReleaseSmoke); err != nil {
			return ReleaseResult{}, err
		}
	}
	if sm.OK {
		return ReleaseResult{Outcome: Released, Previous: prev}, nil
	}
	if prev == "" {
		return ReleaseResult{Outcome: RollbackImpossible, Detail: sm.Detail}, nil
	}
	rb, err := r.start(ctx, job, StepRollback)
	if err != nil {
		return ReleaseResult{}, err
	}
	if !rb.Done {
		if err := r.Deployer.Rollback(ctx, prev); err != nil {
			return ReleaseResult{}, fmt.Errorf("deploy: rollback: %w", err)
		}
		if err := r.Journal.Finish(ctx, job.TaskID, Step{Name: StepRollback, Revision: job.Rev, Previous: prev, OK: true, Detail: sm.Detail}); err != nil {
			return ReleaseResult{}, fmt.Errorf("deploy: %w", err)
		}
	}
	return ReleaseResult{Outcome: RolledBack, Previous: prev, Detail: sm.Detail}, nil
}

// smoke runs a smoke step once: a finished step returns its recorded result. An adapter that
// has no smoke (ErrUnsupported) passes, as a tenant without a smoke command does today.
func (r *Runner) smoke(ctx context.Context, job Job, env deployapi.EnvRef, step string) (deployapi.SmokeResult, error) {
	st, err := r.start(ctx, job, step)
	if err != nil {
		return deployapi.SmokeResult{}, err
	}
	if st.Done {
		return deployapi.SmokeResult{OK: st.OK, Detail: st.Detail}, nil
	}
	res, err := r.Deployer.Smoke(ctx, env)
	if errors.Is(err, deployapi.ErrUnsupported) {
		res, err = deployapi.SmokeResult{OK: true}, nil
	}
	if err != nil {
		return deployapi.SmokeResult{}, fmt.Errorf("deploy: smoke: %w", err)
	}
	if err := r.Journal.Finish(ctx, job.TaskID, Step{Name: step, Revision: job.Rev, OK: res.OK, Detail: res.Detail}); err != nil {
		return deployapi.SmokeResult{}, fmt.Errorf("deploy: %w", err)
	}
	return res, nil
}
