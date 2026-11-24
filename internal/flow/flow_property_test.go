package flow

import (
	"testing"

	"pgregory.net/rapid"
)

var allEvents = []Event{
	LeadStarted, DocsDrafted, DocFindings, DocLoopPassed, Edit, AskReview, Approve, Reject, Comments,
	DepsClosed, DepCanceled, BlockDecided, PROpened, Findings, AllGreen, Returned, VerifierOK, EnvUp,
	EnvFailedCode, InfraFailed, ScenarioFailed, Accepted, RequestChanges, Merge, HandOff, TeamMerged,
	PRClosed, TrainConflict, ReleaseDone, Rollback, RevertMerged, BudgetExhausted, Continue, Revisit, Cancel,
}

var allStates = map[State]bool{
	Idea: true, Grilling: true, DocChecks: true, Spec: true, DesignReview: true, Queued: true,
	Blocked: true, Building: true, Checks: true, Review: true, Preview: true, Acceptance: true,
	HumanReview: true, Ready: true, Merging: true, Released: true, RolledBack: true, HandedOff: true,
	MergedByTeam: true, Escalated: true, Rejected: true, Canceled: true,
}

// T2: invariants hold for any sequence of events (design section 20).
func TestInvariants(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		task := Task{
			State:            Idea,
			Profile:          rapid.SampledFrom([]Profile{Solo, Team}).Draw(t, "profile"),
			NeedsHumanReview: rapid.Bool().Draw(t, "needs_review"),
		}
		for i := range rapid.IntRange(1, 200).Draw(t, "steps") {
			e := rapid.SampledFrom(allEvents).Draw(t, "event")
			task.BudgetExhausted = task.BudgetExhausted || rapid.IntRange(0, 20).Draw(t, "spend") == 0
			before := task
			next, err := Apply(task, e)
			if err != nil {
				if next != before {
					t.Fatalf("step %d: a rejected event changed the task", i)
				}
				continue
			}
			checkStep(t, before, e, next)
			task = next
		}
	})
}

func checkStep(t *rapid.T, before Task, e Event, after Task) {
	switch {
	case !allStates[after.State]:
		t.Fatalf("unknown state %q", after.State)
	case Terminal(before.State):
		t.Fatalf("%s moved terminal %s", e, before.State)
	case after.Profile == Team && (after.State == Merging || after.State == Released || after.State == RolledBack):
		t.Fatalf("team task reached %s: the factory never merges for a team", after.State)
	case after.Profile == Solo && (after.State == HandedOff || after.State == MergedByTeam):
		t.Fatalf("solo task reached %s", after.State)
	case after.Returns > MaxReturns:
		t.Fatalf("returns %d > %d without escalation", after.Returns, MaxReturns)
	case before.BudgetExhausted && returnEvents[e] && after.State == Building:
		t.Fatalf("%s returned to building with an exhausted budget", e)
	case before.State == HandedOff && after.State != MergedByTeam && after.State != Building && after.State != Canceled:
		t.Fatalf("handed_off left by %s to %s: only a GitHub event or a cancel may move it", e, after.State)
	}
}

func TestStricterIsAnOrder(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a := rapid.SampledFrom(allEvents).Draw(t, "a")
		b := rapid.SampledFrom(allEvents).Draw(t, "b")
		c := rapid.SampledFrom(allEvents).Draw(t, "c")
		if Stricter(a, b) != Stricter(b, a) {
			t.Fatalf("not commutative: %s, %s", a, b)
		}
		if Stricter(a, a) != a {
			t.Fatalf("not idempotent: %s", a)
		}
		if Stricter(Stricter(a, b), c) != Stricter(a, Stricter(b, c)) {
			t.Fatalf("not associative: %s, %s, %s", a, b, c)
		}
		if Stricter(a, Cancel) != Cancel {
			t.Fatalf("cancel must win over %s", a)
		}
	})
}
