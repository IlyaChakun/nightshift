package policy

import (
	"errors"
	"fmt"
)

// Mode is the tenant's review switch (design section 11).
type Mode string

// Modes, from the most review to the least.
const (
	ModeAll         Mode = "all"
	ModeBlastRadius Mode = "blast_radius"
	ModeOff         Mode = "off"
)

// Action is what happens to a change at a level.
type Action string

// Actions from schemas/tenant.json.
const (
	ActionAuto            Action = "auto"
	ActionNotify          Action = "notify"
	ActionReview          Action = "review"
	ActionReviewDesignAPR Action = "review_design_and_pr"
)

// NeedsHuman reports whether the action holds the PR for a person.
func (a Action) NeedsHuman() bool { return a == ActionReview || a == ActionReviewDesignAPR }

// ErrTeamOff means a tenant of the team profile asked for `off`, which the design forbids.
var ErrTeamOff = errors.New("policy: a team tenant cannot switch review off")

// defaultActions are the starting values of design section 11.
var defaultActions = map[Level]Action{Low: ActionAuto, Medium: ActionNotify, High: ActionReview, Critical: ActionReviewDesignAPR}

// ReviewPolicy is a tenant's review configuration.
type ReviewPolicy struct {
	Mode    Mode
	Team    bool
	Actions map[Level]Action // overrides of the defaults; critical is not overridable
	// Sample is the share of low and medium changes that still go to the owner, to calibrate.
	Sample float64
}

// Decide returns the action for a change. roll is a number in [0, 1) the caller draws, so the
// function stays deterministic. Fixed by code: `critical` is always reviewed, in every mode.
func (p ReviewPolicy) Decide(level Level, roll float64) (Action, error) {
	if !level.Valid() {
		return "", fmt.Errorf("policy: unknown level %q", level)
	}
	if level == Critical {
		return ActionReviewDesignAPR, nil
	}
	switch p.Mode {
	case ModeAll:
		return ActionReview, nil
	case ModeOff:
		if p.Team {
			return "", ErrTeamOff
		}
		return ActionAuto, nil
	case ModeBlastRadius:
	default:
		return "", fmt.Errorf("policy: unknown review mode %q", p.Mode)
	}
	act, ok := p.Actions[level]
	if !ok {
		act = defaultActions[level]
	}
	if (level == Low || level == Medium) && !act.NeedsHuman() && roll < p.Sample {
		return ActionReview, nil
	}
	return act, nil
}

// StepUp is the automatic move of the autonomy ladder after an escape, revert or rollback:
// toward more review, never beyond `all`. Moving down is only ever the owner's button.
func StepUp(m Mode) Mode {
	if m == ModeOff {
		return ModeBlastRadius
	}
	return ModeAll
}
