package flow

import (
	"errors"
	"slices"
	"testing"
)

func run(t *testing.T, task Task, events ...Event) Task {
	t.Helper()
	if task.State == "" {
		task.State = Idea
	}
	for _, e := range events {
		next, err := Apply(task, e)
		if err != nil {
			t.Fatalf("%s on %s: %v", e, task.State, err)
		}
		task = next
	}
	return task
}

var toBuilding = []Event{LeadStarted, DocsDrafted, DocLoopPassed, Approve, DepsClosed}

func TestPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		task   Task
		events []Event
		want   State
	}{
		{"solo release", Task{Profile: Solo},
			slices.Concat(toBuilding, []Event{PROpened, AllGreen, VerifierOK, EnvUp, Accepted, Merge, ReleaseDone}), Released},
		{"solo with human review", Task{Profile: Solo, NeedsHumanReview: true},
			slices.Concat(toBuilding, []Event{PROpened, AllGreen, VerifierOK, EnvUp, Accepted, Approve}), Ready},
		{"team hand-off", Task{Profile: Team},
			slices.Concat(toBuilding, []Event{PROpened, AllGreen, VerifierOK, EnvUp, Accepted, HandOff, TeamMerged}), MergedByTeam},
		{"team request changes", Task{Profile: Team},
			slices.Concat(toBuilding, []Event{PROpened, AllGreen, VerifierOK, EnvUp, Accepted, HandOff, RequestChanges}), Building},
		{"doc findings loop", Task{}, []Event{LeadStarted, DocsDrafted, DocFindings, DocsDrafted, DocLoopPassed}, Spec},
		{"design review", Task{}, []Event{LeadStarted, DocsDrafted, DocLoopPassed, AskReview, Approve}, Queued},
		{"reject", Task{}, []Event{LeadStarted, DocsDrafted, DocLoopPassed, Reject}, Rejected},
		{"blocked dependency", Task{}, []Event{LeadStarted, DocsDrafted, DocLoopPassed, Approve, DepCanceled, BlockDecided}, Queued},
		{"rollback after release", Task{Profile: Solo},
			slices.Concat(toBuilding, []Event{PROpened, AllGreen, VerifierOK, EnvUp, Accepted, Merge, ReleaseDone, Rollback, RevertMerged}), Building},
		{"train conflict", Task{Profile: Solo},
			slices.Concat(toBuilding, []Event{PROpened, AllGreen, VerifierOK, EnvUp, Accepted, Merge, TrainConflict}), Checks},
		{"fourth return escalates", Task{},
			slices.Concat(toBuilding, []Event{PROpened, AllGreen, Returned, PROpened, AllGreen, Returned, PROpened, AllGreen, Returned,
				PROpened, AllGreen, Returned}), Escalated},
		{"continue after escalation", Task{}, slices.Concat(toBuilding, []Event{BudgetExhausted, Continue}), Building},
		{"cancel while building", Task{}, slices.Concat(toBuilding, []Event{Cancel}), Canceled},
		{"merged outside while waiting for a person", Task{Profile: Solo, NeedsHumanReview: true},
			slices.Concat(toBuilding, []Event{PROpened, AllGreen, VerifierOK, EnvUp, Accepted, MergedOutside}), MergedByTeam},
		{"closed outside while waiting for a person", Task{Profile: Solo, NeedsHumanReview: true},
			slices.Concat(toBuilding, []Event{PROpened, AllGreen, VerifierOK, EnvUp, Accepted, ClosedOutside}), Canceled},
		{"merged outside after approval", Task{Profile: Solo, NeedsHumanReview: true},
			slices.Concat(toBuilding, []Event{PROpened, AllGreen, VerifierOK, EnvUp, Accepted, Approve, MergedOutside}), MergedByTeam},
		{"infrastructure failure", Task{}, slices.Concat(toBuilding, []Event{PROpened, AllGreen, VerifierOK, InfraFailed}), Escalated},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := run(t, tc.task, tc.events...).State; got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestNotAllowed(t *testing.T) {
	t.Parallel()
	ready := Task{State: Ready}
	tests := []struct {
		name string
		task Task
		e    Event
	}{
		{"team cannot merge", Task{State: Ready, Profile: Team}, Merge},
		{"solo does not hand off", Task{State: Ready, Profile: Solo}, HandOff},
		{"no merge without a profile", ready, Merge},
		{"terminal stays terminal", Task{State: Canceled}, LeadStarted},
		{"no cancel after release", Task{State: Released}, Cancel},
		{"no budget outside work", Task{State: Spec}, BudgetExhausted},
		{"approve only where asked", Task{State: Building}, Approve},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Next(tc.task, tc.e); !errors.Is(err, ErrNotAllowed) {
				t.Errorf("got %v, want ErrNotAllowed", err)
			}
		})
	}
}

func TestExhaustedBudgetEscalatesReturns(t *testing.T) {
	t.Parallel()
	got, err := Next(Task{State: Checks, BudgetExhausted: true}, Findings)
	if err != nil || got != Escalated {
		t.Errorf("got %s, %v; want escalated", got, err)
	}
}

func TestStricter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ a, b, want Event }{
		{Approve, RequestChanges, RequestChanges},
		{Merge, Rollback, Rollback},
		{Reject, Approve, Reject},
		{Cancel, Rollback, Cancel},
	} {
		if got := Stricter(tc.a, tc.b); got != tc.want {
			t.Errorf("Stricter(%s, %s) = %s, want %s", tc.a, tc.b, got, tc.want)
		}
	}
}

// A pull request merged or closed on the code host ends the task wherever the task has one.
func TestOutsideMergeEndsATaskInEveryStateWithAPullRequest(t *testing.T) {
	t.Parallel()
	for _, st := range []State{Building, Checks, Review, Preview, Acceptance, HumanReview, Ready} {
		if got, err := Next(Task{State: st, Profile: Solo}, MergedOutside); err != nil || got != MergedByTeam {
			t.Errorf("merged outside in %s: %s, %v", st, got, err)
		}
		if got, err := Next(Task{State: st, Profile: Solo}, ClosedOutside); err != nil || got != Canceled {
			t.Errorf("closed outside in %s: %s, %v", st, got, err)
		}
	}
	// No pull request yet, or the train owns it, or the task is done: not an outside event.
	for _, st := range []State{Idea, Grilling, Queued, Merging, Released, MergedByTeam, Canceled} {
		if _, err := Next(Task{State: st, Profile: Solo}, MergedOutside); err == nil {
			t.Errorf("merged outside must not apply in %s", st)
		}
	}
}
