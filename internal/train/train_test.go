package train_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/train"
)

const (
	oldHead = "1111111111111111111111111111111111111111"
	newHead = "2222222222222222222222222222222222222222"
	mergeID = "3333333333333333333333333333333333333333"
)

type journal struct{ steps map[string]train.Step }

func (j *journal) Start(_ context.Context, _ string, _ int64, step string) (train.Step, error) {
	if s, ok := j.steps[step]; ok {
		s.Fresh = false
		return s, nil
	}
	j.steps[step] = train.Step{Name: step}
	return train.Step{Name: step, Fresh: true}, nil
}

func (j *journal) Finish(_ context.Context, _ int64, step, sha string) error {
	j.steps[step] = train.Step{Name: step, Done: true, SHA: sha}
	return nil
}

func (j *journal) Clear(context.Context, int64) error { j.steps = map[string]train.Step{}; return nil }

type rebaser struct {
	res   gitops.Rebased
	calls int
}

func (r *rebaser) Rebase(context.Context, string, string) (gitops.Rebased, error) {
	r.calls++
	return r.res, nil
}

type forge struct {
	merged, closed bool
	ci             integration.CIState
	mergeErr       error
	mergeCalls     int
	mergedAt       string
	mergeSHA       string // reported by PRState once merged
}

func (f *forge) PRState(_ context.Context, ref integration.PRRef) (integration.PRState, error) {
	st := integration.PRState{Ref: ref, State: "open", Merged: f.merged}
	if f.merged {
		st.MergeSHA = f.mergeSHA
	}
	if f.merged {
		st.State = "merged"
	} else if f.closed {
		st.State = "closed"
	}
	return st, nil
}

func (f *forge) CI(context.Context, string, string) (integration.CIResult, error) {
	return integration.CIResult{State: f.ci}, nil
}

func (f *forge) Merge(_ context.Context, ref integration.PRRef) (string, error) {
	f.mergeCalls++
	f.mergedAt = ref.HeadSHA
	if f.mergeErr != nil {
		f.merged = true // the merge reached GitHub, the reply was lost
		return "", f.mergeErr
	}
	f.merged = true
	return mergeID, nil
}

type mover struct{ events []flow.Event }

func (m *mover) Apply(_ context.Context, mv flow.Move) (bookdb.BookTask, error) {
	m.events = append(m.events, mv.Event)
	return bookdb.BookTask{}, nil
}

type rig struct {
	tr  *train.Train
	j   *journal
	g   *rebaser
	f   *forge
	mv  *mover
	job train.Job
}

func newRig() *rig {
	r := &rig{j: &journal{steps: map[string]train.Step{}}, g: &rebaser{res: gitops.Rebased{SHA: newHead, Identical: true}}, f: &forge{ci: integration.CISuccess}, mv: &mover{}}
	r.tr = &train.Train{Journal: r.j, Git: r.g, Forge: r.f, Mover: r.mv}
	r.job = train.Job{Tenant: "example-solo", TaskID: 1, Branch: "ns/1", Profile: flow.Solo, PR: integration.PRRef{Repo: "example/app", Number: 5, HeadSHA: oldHead}}
	return r
}

func TestCleanRunMergesAtTheRebasedHead(t *testing.T) {
	t.Parallel()
	r := newRig()
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Merged || !res.ApprovalsKept {
		t.Fatalf("run: %+v %v", res, err)
	}
	if r.f.mergedAt != newHead {
		t.Fatalf("merged %s, the tested head is %s", r.f.mergedAt, newHead)
	}
	if len(r.mv.events) != 1 || r.mv.events[0] != flow.ReleaseDone {
		t.Fatalf("events: %v", r.mv.events)
	}
	for _, s := range []string{train.StepRebase, train.StepCI, train.StepMerge} {
		if !r.j.steps[s].Done {
			t.Errorf("step %s is not done: %+v", s, r.j.steps)
		}
	}
}

func TestConflictAndRedCISendTheTaskBack(t *testing.T) {
	t.Parallel()
	for name, set := range map[string]func(*rig){
		"conflict": func(r *rig) { r.g.res = gitops.Rebased{Conflict: true} },
		"red ci":   func(r *rig) { r.f.ci = integration.CIFailure },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newRig()
			set(r)
			res, err := r.tr.Run(t.Context(), r.job)
			if err != nil || res.Outcome != train.SentBack {
				t.Fatalf("run: %+v %v", res, err)
			}
			if len(r.mv.events) != 1 || r.mv.events[0] != flow.TrainConflict || len(r.j.steps) != 0 || r.f.mergeCalls != 0 {
				t.Fatalf("events %v, journal %+v, merges %d", r.mv.events, r.j.steps, r.f.mergeCalls)
			}
		})
	}
}

func TestPendingCIWaitsAndTheNextRunDoesNotRebaseAgain(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.f.ci = integration.CIPending
	if res, err := r.tr.Run(t.Context(), r.job); err != nil || res.Outcome != train.Waiting {
		t.Fatalf("first run: %+v %v", res, err)
	}
	r.f.ci = integration.CISuccess
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Merged || r.g.calls != 1 {
		t.Fatalf("second run: %+v %v, rebases %d", res, err, r.g.calls)
	}
	if res.ApprovalsKept {
		t.Error("approvals carried over after a restart; the fact that the patch-id was equal is lost")
	}
	if r.f.mergedAt != newHead {
		t.Errorf("merged %s, want the journaled head %s", r.f.mergedAt, newHead)
	}
}

func TestAMergeWithALostReplyIsNotRepeated(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.f.mergeErr = errors.New("connection reset")
	if res, err := r.tr.Run(t.Context(), r.job); err != nil || res.Outcome != train.Merged {
		t.Fatalf("run: %+v %v", res, err)
	}
	if r.f.mergeCalls != 1 {
		t.Fatalf("merge calls: %d", r.f.mergeCalls)
	}
}

func TestRestartAfterTheMergeOnlyReleases(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.f.merged = true
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Merged || r.g.calls != 0 || r.f.mergeCalls != 0 {
		t.Fatalf("run: %+v %v (rebases %d, merges %d)", res, err, r.g.calls, r.f.mergeCalls)
	}
	if len(r.mv.events) != 1 || r.mv.events[0] != flow.ReleaseDone {
		t.Fatalf("events: %v", r.mv.events)
	}
}

func TestACrashedRebaseNeverKeepsApprovals(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.j.steps[train.StepRebase] = train.Step{Name: train.StepRebase} // started, never finished
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Merged || res.ApprovalsKept {
		t.Fatalf("run: %+v %v", res, err)
	}
}

func TestAClosedPullRequestIsNotShipped(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.f.closed = true
	if _, err := r.tr.Run(t.Context(), r.job); !errors.Is(err, train.ErrClosed) {
		t.Fatalf("run: %v", err)
	}
}

func TestAPausedFactoryStartsNothing(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.tr.Paused = func(context.Context) (bool, error) { return true, nil }
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Waiting || r.g.calls != 0 || r.f.mergeCalls != 0 {
		t.Fatalf("run: %+v %v (rebases %d, merges %d)", res, err, r.g.calls, r.f.mergeCalls)
	}
}

func releaseWith(out deploy.ReleaseOutcome, ok bool, calls *int) func(context.Context, train.Job, string) (deploy.ReleaseResult, bool, error) {
	return func(_ context.Context, _ train.Job, sha string) (deploy.ReleaseResult, bool, error) {
		*calls++
		if sha != mergeID {
			panic("release got " + sha + ", want the merge commit")
		}
		return deploy.ReleaseResult{Outcome: out, Detail: "checkout returns 500"}, ok, nil
	}
}

func TestReleaseDeploysTheMergeCommitBeforeTheTaskIsReleased(t *testing.T) {
	t.Parallel()
	r := newRig()
	var calls int
	r.tr.Release = releaseWith(deploy.Released, true, &calls)
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Merged || calls != 1 {
		t.Fatalf("run: %+v %v (deploys %d)", res, err, calls)
	}
	if len(r.mv.events) != 1 || r.mv.events[0] != flow.ReleaseDone {
		t.Fatalf("events: %v", r.mv.events)
	}
}

func TestAFailedSmokeRollsBackAndTheTaskRecordsIt(t *testing.T) {
	t.Parallel()
	r := newRig()
	var calls int
	r.tr.Release = releaseWith(deploy.RolledBack, true, &calls)
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.RolledBack {
		t.Fatalf("run: %+v %v", res, err)
	}
	if len(r.mv.events) != 1 || r.mv.events[0] != flow.ReleaseFailed {
		t.Fatalf("events: %v; one move must record the rollback", r.mv.events)
	}
}

func TestAFailedFirstReleaseEscalatesTheTask(t *testing.T) {
	t.Parallel()
	r := newRig()
	var calls int
	r.tr.Release = releaseWith(deploy.RollbackImpossible, true, &calls)
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Escalated {
		t.Fatalf("run: %+v %v", res, err)
	}
	if len(r.mv.events) != 1 || r.mv.events[0] != flow.InfraFailed {
		t.Fatalf("events: %v; a person must decide", r.mv.events)
	}
}

// After a lost merge reply the release deploys the merge commit, not the head of the branch.
func TestALostMergeReplyStillReleasesTheMergeCommit(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.f.mergeErr = errors.New("connection reset")
	r.f.mergeSHA = mergeID
	var calls int
	r.tr.Release = releaseWith(deploy.Released, true, &calls)
	if res, err := r.tr.Run(t.Context(), r.job); err != nil || res.Outcome != train.Merged || calls != 1 {
		t.Fatalf("run: %+v %v (deploys %d)", res, err, calls)
	}
}

func TestATenantWithoutReleaseIsReleasedAtTheMerge(t *testing.T) {
	t.Parallel()
	r := newRig()
	var calls int
	r.tr.Release = releaseWith(deploy.Released, false, &calls)
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Merged || len(r.mv.events) != 1 {
		t.Fatalf("run: %+v %v events %v", res, err, r.mv.events)
	}
}
