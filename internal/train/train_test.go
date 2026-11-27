package train_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/train"
	"github.com/ilyachakun/nightshift/internal/trustedgit"
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

func (j *journal) Peek(_ context.Context, _ int64, step string) (train.Step, bool, error) {
	s, ok := j.steps[step]
	return s, ok, nil
}

func (j *journal) Finish(_ context.Context, _ int64, step, sha string) error {
	j.steps[step] = train.Step{Name: step, Done: true, SHA: sha}
	return nil
}

func (j *journal) Clear(context.Context, int64) error { j.steps = map[string]train.Step{}; return nil }

type rebaser struct {
	res   trustedgit.Rebased
	calls int
	forge *forge
}

func (r *rebaser) Rebase(context.Context, string, string, string) (trustedgit.Rebased, error) {
	r.calls++
	if !r.res.Conflict {
		r.forge.liveHead = r.res.SHA
	}
	return r.res, nil
}

type forge struct {
	merged, closed bool
	liveHead       string
	ci             integration.CIState
	mergeErr       error
	mergeCalls     int
	mergedAt       string
	mergeSHA       string // reported by PRState once merged
}

func (f *forge) PRState(_ context.Context, ref integration.PRRef) (integration.PRState, error) {
	if f.liveHead != "" {
		ref.HeadSHA = f.liveHead
	}
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
	r := &rig{j: &journal{steps: map[string]train.Step{}}, g: &rebaser{res: trustedgit.Rebased{SHA: newHead, Identical: true}}, f: &forge{ci: integration.CISuccess}, mv: &mover{}}
	r.g.forge = r.f
	r.tr = &train.Train{Journal: r.j, Git: r.g, Forge: r.f, Mover: r.mv}
	r.job = train.Job{Tenant: "example-solo", TaskID: 1, Branch: "ns/1", Profile: flow.Solo, PR: integration.PRRef{Repo: "example/app", Number: 5, HeadSHA: oldHead}}
	return r
}

func TestCleanRunMergesAtTheRebasedHead(t *testing.T) {
	t.Parallel()
	r := newRig()
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Merged {
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

func TestTrainRequiresTheReviewedInputHead(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.f.liveHead = newHead
	if _, err := r.tr.Run(t.Context(), r.job); err == nil || r.g.calls != 0 || r.f.mergeCalls != 0 {
		t.Fatalf("run err = %v, rebases = %d, merges = %d", err, r.g.calls, r.f.mergeCalls)
	}
}

func TestTrainChecksPublicationAuthorityAfterCI(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.job.CanPublish = func(_ context.Context, ref integration.PRRef) (bool, error) {
		return ref.HeadSHA == oldHead, nil
	}
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Waiting || r.f.mergeCalls != 0 {
		t.Fatalf("run = %+v, err = %v, merges = %d", res, err, r.f.mergeCalls)
	}
}

func TestConflictAndRedCISendTheTaskBack(t *testing.T) {
	t.Parallel()
	for name, set := range map[string]func(*rig){
		"conflict": func(r *rig) { r.g.res = trustedgit.Rebased{Conflict: true} },
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

// A rebase that cannot vouch for the patch-id does not merge on the earlier approval: the
// task goes back to a person at the rebased head (design section 11).
func TestARebaseThatMayHaveChangedThePatchNeedsAFreshApproval(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		set        func(*rig)
		head       string
		publishing bool // the old head's review evidence is consulted
	}{
		"changed patch": {func(r *rig) { r.g.res = trustedgit.Rebased{SHA: newHead} }, newHead, true},
		// Started, never finished: the push may have landed before a crash. The host head is
		// what needs the approval, whatever the old head's evidence says.
		"resumed rebase": {func(r *rig) { r.j.steps[train.StepRebase] = train.Step{Name: train.StepRebase} }, oldHead, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newRig()
			tc.set(r)
			asked := false
			r.job.CanPublish = func(context.Context, integration.PRRef) (bool, error) { asked = true; return true, nil }
			res, err := r.tr.Run(t.Context(), r.job)
			if err != nil || res.Outcome != train.Reapproval || res.HeadSHA != tc.head || asked != tc.publishing {
				t.Fatalf("run: %+v %v, publication gate asked %v", res, err, asked)
			}
			if r.f.mergeCalls != 0 || r.j.steps[train.StepRebase].Done {
				t.Fatalf("merges %d, journal %+v", r.f.mergeCalls, r.j.steps)
			}
			if len(r.mv.events) != 1 || r.mv.events[0] != flow.ReviewRequired {
				t.Fatalf("events: %v", r.mv.events)
			}
		})
	}
}

func TestReapprovalUsesTheJobCallbackWithTheRebasedHead(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.g.res = trustedgit.Rebased{SHA: newHead}
	var got []string
	r.job.Reapprove = func(_ context.Context, head string) error { got = append(got, head); return nil }
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Reapproval {
		t.Fatalf("run: %+v %v", res, err)
	}
	if len(got) != 1 || got[0] != newHead || len(r.mv.events) != 0 || r.f.mergeCalls != 0 {
		t.Fatalf("callback heads %v, events %v, merges %d", got, r.mv.events, r.f.mergeCalls)
	}

	r.job.Reapprove = func(context.Context, string) error { return errors.New("book down") }
	if _, err := r.tr.Run(t.Context(), r.job); err == nil || r.f.mergeCalls != 0 {
		t.Fatalf("a failed reapproval must stop the run: %v, merges %d", err, r.f.mergeCalls)
	}
}

// After the person approves the rebased head, the next rebase is a no-op and the train merges.
func TestAnApprovedRebasedHeadMergesOnTheNextRun(t *testing.T) {
	t.Parallel()
	r := newRig()
	r.g.res = trustedgit.Rebased{SHA: newHead}
	if res, err := r.tr.Run(t.Context(), r.job); err != nil || res.Outcome != train.Reapproval {
		t.Fatalf("first run: %+v %v", res, err)
	}
	_ = r.j.Clear(t.Context(), r.job.TaskID) // leaving merging clears the journal (flow.record)
	r.job.PR.HeadSHA = newHead
	r.g.res = trustedgit.Rebased{SHA: newHead, Identical: true}
	res, err := r.tr.Run(t.Context(), r.job)
	if err != nil || res.Outcome != train.Merged || r.f.mergedAt != newHead {
		t.Fatalf("second run: %+v %v, merged %s", res, err, r.f.mergedAt)
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
