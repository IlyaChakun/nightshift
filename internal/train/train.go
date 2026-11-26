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
	"github.com/ilyachakun/nightshift/internal/deploy"
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
	// Peek returns a recorded step without recording one; ok is false when there is none.
	Peek(ctx context.Context, taskID int64, step string) (s Step, ok bool, err error)
	Finish(ctx context.Context, taskID int64, step, sha string) error
	Clear(ctx context.Context, taskID int64) error
}

// Rebaser rebases the task's branch on the base branch and publishes the result, so the pull
// request head becomes the returned commit.
type Rebaser interface {
	Rebase(ctx context.Context, repo, branch, expectedHead string) (gitops.Rebased, error)
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
	// Paused, when set, stops new runs while the factory is paused (settings.Store.IsPaused).
	Paused func(ctx context.Context) (bool, error)
	// Release deploys the merge commit when the tenant has a release capability (design section
	// 17): the bool is false when it has none, and the task is then released at the merge, as
	// before. It continues from the deploy journal, so finish may run it twice.
	Release func(ctx context.Context, job Job, mergeSHA string) (deploy.ReleaseResult, bool, error)
	// Releases says before the merge whether a release will follow it (the repository declares a
	// release target). Nil means the answer is whether Release is set. Quiet hours use it.
	Releases func(ctx context.Context, repo string) (bool, error)
}

// Job is a task at the head of its repository's train.
type Job struct {
	Tenant  string
	TaskID  int64
	Branch  string
	PR      integration.PRRef
	Profile flow.Profile
	// CanPublish rechecks review evidence while the PR is open. Already merged PRs
	// still finish from the journal, even when their review evidence is unavailable.
	CanPublish func(context.Context, integration.PRRef) (bool, error)
	// Reapprove sends the task back to a person when the rebase changed the patch or its
	// identity cannot be confirmed (design section 11). Nil moves the task with ReviewRequired.
	Reapprove func(ctx context.Context, head string) error
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
	// RolledBack: the pull request is merged, the release failed its smoke and the previous
	// revision is live again.
	RolledBack Outcome = "rolled_back"
	// Escalated: the merge is in, the release failed its smoke and there is no earlier revision;
	// the task waits for a person.
	Escalated Outcome = "escalated"
	// Reapproval: the rebase changed the patch, or a resumed rebase cannot vouch for it, so the
	// earlier approval no longer covers the change and the task waits for a person again.
	Reapproval Outcome = "reapproval"
)

// Result is the outcome and the head it refers to. A rebase is journaled as done only when
// it kept the patch-id, so every head that reaches CI or the merge carries its approval.
type Result struct {
	Outcome Outcome
	HeadSHA string
}

// Run advances the job as far as it can.
func (t *Train) Run(ctx context.Context, job Job) (Result, error) {
	if t.Paused != nil {
		paused, err := t.Paused(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("train: %w", err)
		}
		if paused {
			return Result{Outcome: Waiting}, nil
		}
	}
	st, err := t.Forge.PRState(ctx, job.PR)
	if err != nil {
		return Result{}, fmt.Errorf("train: %w", err)
	}
	switch {
	case st.Merged:
		return t.finish(ctx, job, mergeCommit(st, st.Ref.HeadSHA))
	case st.State == "closed":
		return Result{}, ErrClosed
	}
	if res, held, err := t.held(ctx, job, st.Ref); held || err != nil {
		return res, err
	}
	head, back, changed, err := t.rebase(ctx, job)
	if err != nil || back {
		return t.backOrErr(ctx, job, back, err)
	}
	if changed {
		return t.reapprove(ctx, job, head)
	}
	ref := job.PR
	ref.HeadSHA = head
	state, err := t.ci(ctx, job, ref)
	if err != nil {
		return Result{}, err
	}
	switch state {
	case integration.CIPending:
		return Result{Outcome: Waiting, HeadSHA: head}, nil
	case integration.CISuccess:
	default:
		return t.sendBack(ctx, job)
	}
	current, err := t.Forge.PRState(ctx, ref)
	if err != nil {
		return Result{}, fmt.Errorf("train: confirm merge head: %w", err)
	}
	if current.Merged {
		return t.finish(ctx, job, mergeCommit(current, current.Ref.HeadSHA))
	}
	if current.State == "closed" {
		return Result{}, ErrClosed
	}
	if current.Ref.HeadSHA != head {
		return Result{}, errors.New("train: PR head changed before merge")
	}
	if res, held, err := t.held(ctx, job, current.Ref); held || err != nil {
		return res, err
	}
	merged, err := t.merge(ctx, job, ref)
	if err != nil {
		return Result{}, err
	}
	return t.finish(ctx, job, merged)
}

// held reports whether the PR must not go further in this run: a rebase that never finished
// left a head no approval covers yet, or its review evidence does not allow publication.
func (t *Train) held(ctx context.Context, job Job, host integration.PRRef) (Result, bool, error) {
	// A rebase that started earlier may have pushed a head no approval covers, so the review
	// evidence of the old head cannot decide: a person looks at what the code host now has.
	rb, ok, err := t.Journal.Peek(ctx, job.TaskID, StepRebase)
	if err != nil {
		return Result{}, true, fmt.Errorf("train: %w", err)
	}
	if host.HeadSHA == "" || job.PR.HeadSHA == "" {
		return Result{}, true, errors.New("train: missing publication head")
	}
	if ok && !rb.Done {
		res, err := t.reapprove(ctx, job, host.HeadSHA)
		return res, true, err
	}
	expected := job.PR.HeadSHA
	if ok {
		expected = rb.SHA
	}
	if host.HeadSHA != expected {
		return Result{}, true, errors.New("train: PR head awaits reconciliation")
	}
	if job.CanPublish == nil {
		return Result{}, false, nil
	}
	allowed, err := job.CanPublish(ctx, host)
	if err != nil {
		return Result{}, true, fmt.Errorf("train: publication gate: %w", err)
	}
	return Result{Outcome: Waiting}, !allowed, nil
}

func (t *Train) backOrErr(ctx context.Context, job Job, back bool, err error) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	if back {
		return t.sendBack(ctx, job)
	}
	return Result{}, nil
}

// rebase runs the first step, or reads its result from the journal. back is true on a conflict.
//
// changed is true when the approval given before the rebase no longer covers the head.
func (t *Train) rebase(ctx context.Context, job Job) (head string, back, changed bool, err error) {
	rb, err := t.Journal.Start(ctx, job.Tenant, job.TaskID, StepRebase)
	if err != nil {
		return "", false, false, fmt.Errorf("train: %w", err)
	}
	if rb.Done {
		return rb.SHA, false, false, nil
	}
	res, err := t.Git.Rebase(ctx, job.PR.Repo, job.Branch, job.PR.HeadSHA)
	if err != nil {
		return "", false, false, fmt.Errorf("train: rebase: %w", err)
	}
	if res.Conflict {
		return "", true, false, nil
	}
	// A rebase that crashed half way may have done the work already, and a second one then
	// looks like a no-op: only a rebase that started in this run can vouch for the patch-id.
	// Any other rebase is not journaled as done, so a crash before the task leaves the train
	// repeats this decision instead of merging on an approval that no longer applies.
	if !res.Identical || !rb.Fresh {
		return res.SHA, false, true, nil
	}
	if err := t.Journal.Finish(ctx, job.TaskID, StepRebase, res.SHA); err != nil {
		return "", false, false, fmt.Errorf("train: %w", err)
	}
	return res.SHA, false, false, nil
}

// reapprove hands the task back to a person; moving it out of merging clears the journal.
func (t *Train) reapprove(ctx context.Context, job Job, head string) (Result, error) {
	if head == "" {
		return Result{}, errors.New("train: reapproval without a head")
	}
	if job.Reapprove != nil {
		if err := job.Reapprove(ctx, head); err != nil {
			return Result{}, fmt.Errorf("train: reapproval: %w", err)
		}
		return Result{Outcome: Reapproval, HeadSHA: head}, nil
	}
	if _, err := t.Mover.Apply(ctx, flow.Move{TaskID: job.TaskID, Event: flow.ReviewRequired, Actor: "system", Profile: job.Profile}); err != nil {
		return Result{}, fmt.Errorf("train: reapproval: %w", err)
	}
	return Result{Outcome: Reapproval, HeadSHA: head}, nil
}

// ci reads CI of the rebased head and journals the step once it is green.
func (t *Train) ci(ctx context.Context, job Job, ref integration.PRRef) (integration.CIState, error) {
	if _, err := t.Journal.Start(ctx, job.Tenant, job.TaskID, StepCI); err != nil {
		return "", fmt.Errorf("train: %w", err)
	}
	ci, err := t.Forge.CI(ctx, ref.Repo, ref.HeadSHA)
	if err != nil {
		return "", fmt.Errorf("train: ci: %w", err)
	}
	if ci.State == integration.CISuccess {
		if err := t.Journal.Finish(ctx, job.TaskID, StepCI, ref.HeadSHA); err != nil {
			return "", fmt.Errorf("train: %w", err)
		}
	}
	return ci.State, nil
}

// merge squashes the pull request at the tested head.
func (t *Train) merge(ctx context.Context, job Job, ref integration.PRRef) (string, error) {
	if _, err := t.Journal.Start(ctx, job.Tenant, job.TaskID, StepMerge); err != nil {
		return "", fmt.Errorf("train: %w", err)
	}
	merged, err := t.Forge.Merge(ctx, ref)
	if err != nil {
		// A merge that reached GitHub before a crash or a lost reply shows as merged now.
		cur, serr := t.Forge.PRState(ctx, ref)
		if serr != nil || !cur.Merged {
			return "", fmt.Errorf("train: merge: %w", err)
		}
		return mergeCommit(cur, ref.HeadSHA), nil
	}
	return merged, nil
}

// mergeCommit is the commit on the base branch that a release deploys; the head of the branch
// stands in only when the code host did not say.
func mergeCommit(st integration.PRState, head string) string {
	if st.MergeSHA != "" {
		return st.MergeSHA
	}
	return head
}

// finish records the merge and releases the task. It is safe to run twice: a task that
// already left merging is a no-op.
func (t *Train) finish(ctx context.Context, job Job, sha string) (Result, error) {
	if err := t.Journal.Finish(ctx, job.TaskID, StepMerge, sha); err != nil {
		return Result{}, fmt.Errorf("train: %w", err)
	}
	ev, out := flow.ReleaseDone, Merged
	if t.Release != nil {
		rel, ok, err := t.Release(ctx, job, sha)
		if err != nil {
			return Result{}, fmt.Errorf("train: deploy: %w", err)
		}
		switch {
		case ok && rel.Outcome == deploy.RolledBack:
			// The automatic rollback already put the old revision live again. One move records it, so
			// a crash cannot leave the task released while production runs the old revision.
			ev, out = flow.ReleaseFailed, RolledBack
		case ok && rel.Outcome == deploy.RollbackImpossible:
			// The first release failed its smoke and there is nothing to return to: a person decides.
			ev, out = flow.InfraFailed, Escalated
		}
	}
	if _, err := t.Mover.Apply(ctx, flow.Move{TaskID: job.TaskID, Event: ev, Actor: "system", Profile: job.Profile}); err != nil && !errors.Is(err, flow.ErrNotAllowed) {
		return Result{}, fmt.Errorf("train: release: %w", err)
	}
	return Result{Outcome: out, HeadSHA: sha}, nil
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
