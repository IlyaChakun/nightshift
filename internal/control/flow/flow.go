// Package flow is the task state machine of design section 7. It is pure: it decides the
// next state, and the book applies it with a compare-and-set on (state, version, head_sha).
package flow

import (
	"errors"
	"fmt"
)

// State of a task. The values are stored in book.tasks.state.
type State string

// States from the diagram in design section 7.
const (
	Idea         State = "idea"
	Grilling     State = "grilling"
	DocChecks    State = "doc_checks"
	Spec         State = "spec"
	DesignReview State = "design_review"
	Queued       State = "queued"
	Blocked      State = "blocked"
	Building     State = "building"
	Checks       State = "checks"
	Review       State = "review"
	Preview      State = "preview"
	Acceptance   State = "acceptance"
	HumanReview  State = "human_review"
	Ready        State = "ready"
	Merging      State = "merging"
	Released     State = "released"
	RolledBack   State = "rolled_back"
	HandedOff    State = "handed_off"
	MergedByTeam State = "merged_by_team"
	Escalated    State = "escalated"
	Rejected     State = "rejected"
	Canceled     State = "canceled"
)

// Event moves a task. Decisions from people and results from roles both arrive as events.
type Event string

// Events of design section 7.
const (
	LeadStarted    Event = "lead_started"
	DocsDrafted    Event = "docs_drafted"
	DocFindings    Event = "doc_findings"
	DocLoopPassed  Event = "doc_loop_passed"
	Edit           Event = "edit"
	AskReview      Event = "review"
	Approve        Event = "approve"
	Reject         Event = "reject"
	Comments       Event = "comments"
	DepsClosed     Event = "deps_closed"
	DepCanceled    Event = "dep_canceled"
	BlockDecided   Event = "block_decided"
	PROpened       Event = "pr_opened"
	Findings       Event = "findings"
	AllGreen       Event = "all_green"
	Returned       Event = "returned"
	VerifierOK     Event = "verifier_ok"
	EnvUp          Event = "env_up"
	EnvFailedCode  Event = "env_failed_code"
	InfraFailed    Event = "infra_failed"
	ScenarioFailed Event = "scenario_failed"
	Accepted       Event = "accepted"
	ReviewRequired Event = "review_required"
	RequestChanges Event = "request_changes"
	Merge          Event = "merge"
	HandOff        Event = "hand_off"
	TeamMerged     Event = "team_merged"
	PRClosed       Event = "pr_closed"
	// VerifierUnavailable keeps the review distinct from a pass in the task history.
	VerifierUnavailable Event = "verifier_unavailable"
	// MergedOutside and ClosedOutside: someone merged or closed the pull request on the code
	// host while the task waited for a person, so the task follows what happened there.
	MergedOutside Event = "merged_outside"
	ClosedOutside Event = "closed_outside"
	TrainConflict Event = "train_conflict"
	ReleaseDone   Event = "release_done"
	Rollback      Event = "rollback"
	// ReleaseFailed: the merge was released, its smoke failed and the previous revision is live
	// again; the task goes straight to rolled_back.
	ReleaseFailed   Event = "release_failed"
	RevertMerged    Event = "revert_merged"
	BudgetExhausted Event = "budget_exhausted"
	Continue        Event = "continue"
	Revisit         Event = "revisit"
	// ResearchNeeded: Builder stopped because the plan needs a choice the spec does not cover;
	// the task goes back to Lead (design section 6, "Research before building").
	ResearchNeeded Event = "research_needed"
	Cancel         Event = "cancel"
)

// Profile is the tenant's governance: solo merges itself, team hands the PR to people.
type Profile string

// Profiles from schemas/tenant.json.
const (
	Solo Profile = "solo"
	Team Profile = "team"
)

// ProfileFor is the profile of a tenant's `governance` setting: "team" or, by default, solo.
func ProfileFor(governance string) Profile {
	if governance == "team" {
		return Team
	}
	return Solo
}

// MaxReturns is how many times review may send a task back to Builder; the next one escalates.
const MaxReturns = 3

// Task is what the machine needs to know about a task.
type Task struct {
	State   State
	Profile Profile
	Returns int // review returns so far
	// NeedsHumanReview is the review policy's verdict for this task (design section 11).
	NeedsHumanReview bool
	// BudgetExhausted is true once iterations, tokens or hours are used up.
	BudgetExhausted bool
}

// ErrNotAllowed means the event does not apply in the task's state.
var ErrNotAllowed = errors.New("flow: transition not allowed")

type edge struct {
	from State
	on   Event
}

var edges = map[edge]State{
	{Idea, LeadStarted}:           Grilling,
	{Grilling, DocsDrafted}:       DocChecks,
	{DocChecks, DocFindings}:      Grilling,
	{DocChecks, DocLoopPassed}:    Spec,
	{Spec, Edit}:                  Grilling,
	{Spec, AskReview}:             DesignReview,
	{Spec, Approve}:               Queued,
	{Spec, Reject}:                Rejected,
	{DesignReview, Comments}:      Grilling,
	{DesignReview, Approve}:       Queued,
	{Queued, DepsClosed}:          Building,
	{Queued, DepCanceled}:         Blocked,
	{Blocked, BlockDecided}:       Queued,
	{Building, PROpened}:          Checks,
	{Building, ResearchNeeded}:    Grilling,
	{Checks, Findings}:            Building,
	{Checks, AllGreen}:            Review,
	{Review, Returned}:            Building,
	{Review, VerifierOK}:          Preview,
	{Review, VerifierUnavailable}: Preview,
	{Preview, EnvUp}:              Acceptance,
	{Preview, EnvFailedCode}:      Building,
	{Preview, InfraFailed}:        Escalated,
	{Acceptance, ScenarioFailed}:  Building,
	{Acceptance, Accepted}:        Ready, // or HumanReview, see Next
	{HumanReview, RequestChanges}: Building,
	{HumanReview, Approve}:        Ready,
	{Ready, Merge}:                Merging,
	{Ready, HandOff}:              HandedOff,
	{Ready, ReviewRequired}:       HumanReview,
	{Ready, Rollback}:             Canceled,
	{HandedOff, TeamMerged}:       MergedByTeam,
	{HandedOff, RequestChanges}:   Building,
	{HandedOff, PRClosed}:         Canceled,
	{Merging, TrainConflict}:      Checks,
	{Merging, ReviewRequired}:     HumanReview,
	{Merging, ReleaseDone}:        Released,
	{Merging, ReleaseFailed}:      RolledBack,
	{Merging, InfraFailed}:        Escalated,
	{Released, Rollback}:          RolledBack,
	{RolledBack, RevertMerged}:    Building,
	{Escalated, Continue}:         Building,
	{Escalated, Revisit}:          Grilling,
	{Escalated, Cancel}:           Canceled,
	{Blocked, Cancel}:             Canceled,
}

// HasPR says whether a task in this state may have an open pull request that someone else can
// merge or close: from the first push on until the train takes over.
func HasPR(s State) bool {
	switch s { //nolint:exhaustive // the other states have no pull request, or the train owns it
	case Building, Checks, Review, Preview, Acceptance, HumanReview, Ready:
		return true
	}
	return false
}

// Terminal reports whether no event can move the task any more.
func Terminal(s State) bool {
	return s == Rejected || s == Canceled || s == MergedByTeam
}

// returnEvents send work back to Builder; with an exhausted budget they escalate instead.
var returnEvents = map[Event]bool{
	Findings: true, Returned: true, EnvFailedCode: true, ScenarioFailed: true,
	RequestChanges: true, TrainConflict: true,
}

// working states spend the task budget.
var working = map[State]bool{
	Building: true, Checks: true, Review: true, Preview: true, Acceptance: true, Merging: true,
}

// Next returns the state after the event, or ErrNotAllowed.
func Next(t Task, e Event) (State, error) {
	if Terminal(t.State) {
		return "", fmt.Errorf("%w: %s is terminal", ErrNotAllowed, t.State)
	}
	// Cancel works from any active state (the tracker or /ns cancel), except after release,
	// where only a rollback undoes the change.
	if e == Cancel && t.State != Released && t.State != RolledBack {
		return Canceled, nil
	}
	// A pull request merged or closed on the code host ends the task wherever it waits for it.
	if (e == MergedOutside || e == ClosedOutside) && HasPR(t.State) {
		if e == MergedOutside {
			return MergedByTeam, nil
		}
		return Canceled, nil
	}
	if e == BudgetExhausted {
		if !working[t.State] {
			return "", fmt.Errorf("%w: no budget is spent in %s", ErrNotAllowed, t.State)
		}
		return Escalated, nil
	}
	to, ok := edges[edge{t.State, e}]
	if !ok {
		return "", fmt.Errorf("%w: %s on %s", ErrNotAllowed, e, t.State)
	}
	switch {
	case e == Merge && t.Profile != Solo:
		return "", fmt.Errorf("%w: only the solo profile merges", ErrNotAllowed)
	case e == HandOff && t.Profile != Team:
		return "", fmt.Errorf("%w: only the team profile hands off", ErrNotAllowed)
	case e == Returned && t.Returns >= MaxReturns:
		return Escalated, nil
	case returnEvents[e] && t.BudgetExhausted:
		return Escalated, nil
	case e == Accepted && t.NeedsHumanReview:
		return HumanReview, nil
	}
	return to, nil
}

// Apply returns the task after the event: the new state and the updated return counter.
// Continuing or revisiting an escalated task starts a fresh budget and return count.
func Apply(t Task, e Event) (Task, error) {
	to, err := Next(t, e)
	if err != nil {
		return t, err
	}
	switch {
	case (e == Returned || e == ScenarioFailed) && to == Building:
		t.Returns++
	case t.State == Escalated && (e == Continue || e == Revisit):
		t.Returns = 0
		t.BudgetExhausted = false
	}
	t.State = to
	return t, nil
}

// strictness orders conflicting decisions that arrive at once: the stricter one wins
// (design section 7), for example Request changes in GitHub over Approve in chat.
var strictness = map[Event]int{
	Merge: 1, HandOff: 1, Approve: 2, AskReview: 3, Comments: 4, Edit: 4,
	ReviewRequired: 5, RequestChanges: 5, Reject: 6, Rollback: 7, Cancel: 8,
}

// Stricter returns the stricter of two decisions; unknown events lose to known ones.
func Stricter(a, b Event) Event {
	if strictness[b] > strictness[a] || (strictness[b] == strictness[a] && b < a) {
		return b
	}
	return a
}
