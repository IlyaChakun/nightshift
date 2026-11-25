package dispatch

import (
	"encoding/json"
	"fmt"

	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/policy"
)

// Verdict is the part of a Verifier result (schemas/verdict.json) the conductor acts on.
type Verdict struct {
	Verdict string `json:"verdict"`
	Summary string `json:"summary"`
	Raise   *struct {
		Level  policy.Level `json:"level"`
		Reason string       `json:"reason"`
	} `json:"raise_blast_radius"`
	EscalationReason string `json:"escalation_reason"`
}

// Outcome is what the conductor does with a verdict.
type Outcome struct {
	Event  flow.Event   // the flow event to apply
	Why    string       // set when the task escalates
	Raised policy.Level // the blast radius after the verdict, never below current
}

// Decide turns a Verifier result into a flow event. A return counts against maxReturns; past
// it the task escalates, because a third identical loop is a design problem, not a fix. An
// unknown verdict escalates too: the safe side is a person. The agent may raise the blast
// radius but never lower it (design section 11).
func Decide(raw []byte, current policy.Level, returnsSoFar, maxReturns int) (Outcome, error) {
	var v Verdict
	if err := json.Unmarshal(raw, &v); err != nil {
		return Outcome{}, fmt.Errorf("verdict: not JSON: %w", err)
	}
	out := Outcome{Raised: current}
	if v.Raise != nil && v.Raise.Level.Valid() {
		out.Raised = policy.Max(current, v.Raise.Level)
	}
	switch v.Verdict {
	case "pass":
		out.Event = flow.VerifierOK
	case "return":
		if returnsSoFar >= maxReturns {
			out.Event, out.Why = flow.BudgetExhausted, fmt.Sprintf("the Verifier returned the work %d times already: %s", returnsSoFar, v.Summary)
		} else {
			out.Event = flow.Returned
		}
	case "escalate":
		out.Event, out.Why = flow.BudgetExhausted, "the Verifier escalated: "+v.EscalationReason
	default:
		out.Event, out.Why = flow.BudgetExhausted, fmt.Sprintf("unknown Verifier verdict %q", v.Verdict)
	}
	return out, nil
}
