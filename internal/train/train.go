// Package train is the merge train (design section 7): one pull request per repository at a
// time goes through rebase on a fresh main, CI on the rebased head and merge. Every step is
// written to the journal before it runs and again when it is done, so after a restart Run
// finds where the train stopped and asks GitHub what really happened instead of repeating a
// merge. The release and smoke steps come with phase 6; until then a merged task is released.
package train

import (
	"context"
	"errors"
	"fmt"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// Journal steps, in order.
const (
	StepRebase = "rebase"
	StepCI     = "ci"
	StepMerge  = "merge"
)

// ErrClosed means the pull request was closed without a merge, so there is nothing to ship.
var ErrClosed = errors.New("train: the pull request was closed without a merge")

// Step is one journal row.
type Step struct {
	Name string
	Done bool
	SHA  string
	// Fresh is true when this call created the row; false means an earlier run, possibly
	// one that crashed half way, already started the step.
	Fresh bool
}

// Journal stores the train's steps in the book.
type Journal interface {
	// Start records a step before it runs. A step that is already there is returned as it is,
	// with Fresh false.
	Start(ctx context.Context, tenant string, taskID int64, step string) (Step, error)
	Finish(ctx context.Context, taskID int64, step, sha string) error
	Clear(ctx context.Context, taskID int64) error
}

// Rebaser rebases the task's branch on the base branch and publishes the result, so the pull
// request head becomes the returned commit.
type Rebaser interface {
	Rebase(ctx context.Context, repo, branch string) (gitops.Rebased, error)
}

// Forge is the part of the code host the train uses; integration.PullRequests implements it.
type Forge interface {
	PRState(ctx context.Context, ref integration.PRRef) (integration.PRState, error)
	CI(ctx context.Context, repo, sha string) (integration.CIResult, error)
	Merge(ctx context.Context, ref integration.PRRef) (string, error)
}

// Mover moves the task in the book; flow.Mover implements it.
type Mover interface {
	Apply(ctx context.Context, mv flow.Move) (bookdb.BookTask, error)
}

// Train ships tasks. Run it for one task per repository at a time.
type Train struct {
	Journal Journal
	Git     Rebaser
	Forge   Forge
	Mover   Mover
}

// Job is a task at the head of its repository's train.
type Job struct {
	Tenant  string
	TaskID  int64
	Branch  string
	PR      integration.PRRef
	Profile flow.Profile
}

// Outcome says how a run ended.
type Outcome string

// Outcomes of Run.
const (
	// Merged: the pull request is merged and the task released.
	Merged Outcome = "merged"
	// Waiting: CI on the rebased head has not finished; call Run again later.
	Waiting Outcome = "waiting"
	// SentBack: a conflict or red CI sent the task back to checks; the journal is cleared.
	SentBack Outcome = "sent_back"
)

// Result is the outcome and, when the rebase ran in this call, whether the change is
// identical to what was approved. After a restart that fact is lost and approvals are
// dropped, which is the safe side.
type Result struct {
	Outcome Outcome
	// ApprovalsKept is true when the rebase was clean and the patch-id did not change.
	ApprovalsKept bool
	HeadSHA       string
}

// Run advances the job as far as it can.
func (t *Train) Run(ctx context.Context, job Job) (Result, error) {
	st, err := t.Forge.PRState(ctx, job.PR)
	if err != nil {
		return Result{}, fmt.Errorf("train: %w", err)
	}
	switch {
	case st.Merged:
		return t.finish(ctx, job, st.Ref.HeadSHA)
	case st.State == "closed":
		return Result{}, ErrClosed
	}

	head, kept := job.PR.HeadSHA, false
	rb, err := t.Journal.Start(ctx, job.Tenant, job.TaskID, StepRebase)
	if err != nil {
		return Result{}, fmt.Errorf("train: %w", err)
	}
	if rb.Done {
		head = rb.SHA
	} else {
		res, err := t.Git.Rebase(ctx, job.PR.Repo, job.Branch)
		if err != nil {
			return Result{}, fmt.Errorf("train: rebase: %w", err)
		}
		if res.Conflict {
			return t.sendBack(ctx, job)
		}
		// A rebase that crashed half way may have done the work already, and a second one then
		// looks like a no-op: only a rebase that started in this run can vouch for the patch-id.
		head, kept = res.SHA, res.Identical && rb.Fresh
		if err := t.Journal.Finish(ctx, job.TaskID, StepRebase, head); err != nil {
			return Result{}, fmt.Errorf("train: %w", err)
		}
	}
	ref := job.PR
	ref.HeadSHA = head

	if _, err := t.Journal.Start(ctx, job.Tenant, job.TaskID, StepCI); err != nil {
		return Result{}, fmt.Errorf("train: %w", err)
	}
	ci, err := t.Forge.CI(ctx, ref.Repo, head)
	if err != nil {
		return Result{}, fmt.Errorf("train: ci: %w", err)
	}
	switch ci.State {
	case integration.CIPending:
		return Result{Outcome: Waiting, ApprovalsKept: kept, HeadSHA: head}, nil
	case integration.CISuccess:
	default:
		return t.sendBack(ctx, job)
	}
	if err := t.Journal.Finish(ctx, job.TaskID, StepCI, head); err != nil {
		return Result{}, fmt.Errorf("train: %w", err)
	}

	if _, err := t.Journal.Start(ctx, job.Tenant, job.TaskID, StepMerge); err != nil {
		return Result{}, fmt.Errorf("train: %w", err)
	}
	merged, err := t.Forge.Merge(ctx, ref)
	if err != nil {
		// A merge that reached GitHub before a crash or a lost reply shows as merged now.
		if cur, serr := t.Forge.PRState(ctx, ref); serr != nil || !cur.Merged {
			return Result{}, fmt.Errorf("train: merge: %w", err)
		}
		merged = head
	}
	res, err := t.finish(ctx, job, merged)
	res.ApprovalsKept = kept
	return res, err
}

// finish records the merge and releases the task. It is safe to run twice: a task that
// already left merging is a no-op.
func (t *Train) finish(ctx context.Context, job Job, sha string) (Result, error) {
	if err := t.Journal.Finish(ctx, job.TaskID, StepMerge, sha); err != nil {
		return Result{}, fmt.Errorf("train: %w", err)
	}
	_, err := t.Mover.Apply(ctx, flow.Move{TaskID: job.TaskID, Event: flow.ReleaseDone, Actor: "system", Profile: job.Profile})
	if err != nil && !errors.Is(err, flow.ErrNotAllowed) {
		return Result{}, fmt.Errorf("train: release: %w", err)
	}
	return Result{Outcome: Merged, HeadSHA: sha}, nil
}

// sendBack returns the task to checks after a conflict or red CI and starts a fresh run later.
func (t *Train) sendBack(ctx context.Context, job Job) (Result, error) {
	if err := t.Journal.Clear(ctx, job.TaskID); err != nil {
		return Result{}, fmt.Errorf("train: %w", err)
	}
	if _, err := t.Mover.Apply(ctx, flow.Move{TaskID: job.TaskID, Event: flow.TrainConflict, Actor: "system", Profile: job.Profile}); err != nil {
		return Result{}, fmt.Errorf("train: send back: %w", err)
	}
	return Result{Outcome: SentBack}, nil
}
