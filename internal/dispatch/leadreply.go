package dispatch

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ilyachakun/nightshift/internal/config"
)

// maxReplyWords is the length of a Lead message (design section 7).
const maxReplyWords = 60

// maxBasisWords bounds the evidence line of a card, so a card with a basis stays short.
const maxBasisWords = 25

// LeadBasis is the evidence behind a reply: what Lead opened, what it only inferred, and how sure it is.
type LeadBasis struct {
	Checked    []string `json:"checked"`
	Inferred   []string `json:"inferred"`
	Confidence string   `json:"confidence"`
}

// maxUnderstoodWords bounds the "how I understood it" line of a card.
const maxUnderstoodWords = 25

// LeadReply is a validated Lead chat message.
type LeadReply struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Recommended *int   `json:"recommended"`
	Options     []struct {
		Label string `json:"label"`
	} `json:"options"`
	Basis *LeadBasis `json:"basis"`
	// Understood is Lead's reading of the task in its own words; the owner sees it on question and spec cards.
	Understood string `json:"understood"`
	SpecReady  bool   `json:"spec_ready"`
	// DuplicateOf is the number of an earlier task that is the same request.
	DuplicateOf int64 `json:"duplicate_of"`
	// HiddenScenarios are the acceptance scenarios Builder must not see; only a finished spec carries them.
	HiddenScenarios []json.RawMessage `json:"hidden_scenarios"`
	Documents       []json.RawMessage `json:"documents"`
}

// CheckLeadReply validates a Lead result against the schema and the rules a schema cannot
// state: at most 60 words, a decision comes with options and a recommendation that points at
// one of them, and a plain message carries no options. A reply that fails goes back to Lead
// once; it never reaches the owner as it is.
func CheckLeadReply(v *config.Validator, raw []byte) (LeadReply, error) {
	if err := v.ValidateJSON(config.KindLeadReply, raw); err != nil {
		return LeadReply{}, fmt.Errorf("lead reply: %w", err)
	}
	var r LeadReply
	if err := json.Unmarshal(raw, &r); err != nil {
		return LeadReply{}, fmt.Errorf("lead reply: %w", err)
	}
	if n := len(strings.Fields(r.Text)); n > maxReplyWords {
		return LeadReply{}, fmt.Errorf("lead reply: %d words, the limit is %d", n, maxReplyWords)
	}
	if err := checkParts(r); err != nil {
		return LeadReply{}, err
	}
	if err := checkUnderstood(r); err != nil {
		return LeadReply{}, err
	}
	if err := checkBasis(r); err != nil {
		return LeadReply{}, err
	}
	switch r.Type {
	case "question", "decision_needed":
		if r.Type == "decision_needed" && len(r.Options) < 2 {
			return LeadReply{}, errors.New("lead reply: a decision needs two or three options")
		}
		if r.Recommended != nil && *r.Recommended >= len(r.Options) {
			return LeadReply{}, errors.New("lead reply: the recommendation points at no option")
		}
	default:
		if len(r.Options) > 0 {
			return LeadReply{}, fmt.Errorf("lead reply: a %s message carries no options", r.Type)
		}
	}
	return r, nil
}

// checkBasis enforces the evidence rule (AGENTS.md "Evidence"): a recommendation or a spec carries
// a basis that names at least one thing Lead opened, and a low-confidence recommendation goes back
// so Lead checks before it advises.
func checkBasis(r LeadReply) error {
	if r.Basis == nil && r.Recommended == nil && !r.SpecReady {
		return nil
	}
	if r.Basis == nil {
		return errors.New("lead reply: a recommendation or a spec needs a basis (what was checked, what was inferred, confidence)")
	}
	if len(r.Basis.Checked) == 0 {
		return errors.New("lead reply: the basis names nothing that was checked; open the source first")
	}
	// Low confidence is honest and allowed in a plain message that recommends nothing; it
	// is the only way left to say "I could not check this".
	if r.Basis.Confidence == "low" && (r.Recommended != nil || r.SpecReady) {
		return errors.New("lead reply: confidence is low; check the source first, or send a blocker_or_summary without a recommendation or a spec that says what you could not check")
	}
	if n := len(strings.Fields(basisLine(r.Basis))); n > maxBasisWords {
		return fmt.Errorf("lead reply: the basis is %d words, the limit is %d", n, maxBasisWords)
	}
	return nil
}

// basisLine renders the basis as one short line, in English: the length check counts it.
func basisLine(b *LeadBasis) string { return basisLineIn(b, labelsFor(false)) }

// basisLineIn renders the basis as one short line for the card, with the card's labels.
func basisLineIn(b *LeadBasis, l cardLabels) string {
	if b == nil {
		return ""
	}
	line := l.Checked + ": " + strings.Join(b.Checked, "; ")
	if len(b.Inferred) > 0 {
		line += ". " + l.Inferred + ": " + strings.Join(b.Inferred, "; ")
	}
	return line + ". " + l.Confidence + ": " + l.level(b.Confidence)
}

// checkUnderstood requires the restating line on a question, a decision and a spec, so a wrong
// reading shows on the card before work starts (docs/research/task-intake.md).
func checkUnderstood(r LeadReply) error {
	needed := r.SpecReady || r.Type == "question" || r.Type == "decision_needed"
	n := len(strings.Fields(r.Understood))
	switch {
	case needed && n == 0:
		return errors.New("lead reply: a question, a decision or a spec carries `understood`: your reading of the task in one line")
	case n > maxUnderstoodWords:
		return fmt.Errorf("lead reply: `understood` is %d words, the limit is %d", n, maxUnderstoodWords)
	}
	return nil
}

// checkParts refuses the parts of a reply that belong only to a finished spec or a duplicate.
func checkParts(r LeadReply) error {
	switch {
	case r.DuplicateOf > 0 && (r.SpecReady || r.Type != "blocker_or_summary"):
		return errors.New("lead reply: a duplicate is a blocker_or_summary message without a spec")
	case len(r.HiddenScenarios) > 0 && !r.SpecReady:
		return errors.New("lead reply: hidden scenarios come with a finished spec")
	case len(r.Documents) > 0 && !r.SpecReady:
		return errors.New("lead reply: documents come with a finished spec")
	}
	return nil
}

// CheckExpect grades what Lead did with an eval case that says what a good answer is: "ask" is a
// question or a decision and no spec, "spec" is a finished spec and no question, "answer" is a plain answer with no question and no spec. Empty grades
// nothing; any other value is an error in the case.
func CheckExpect(expect string, r LeadReply) error {
	asked := r.Type == "question" || r.Type == "decision_needed"
	switch expect {
	case "":
		return nil
	case "ask":
		if !asked || r.SpecReady {
			return fmt.Errorf("expected a question, got a %s reply (spec_ready=%t)", r.Type, r.SpecReady)
		}
	case "answer":
		if asked || r.SpecReady || r.Type != "blocker_or_summary" {
			return fmt.Errorf("expected a plain answer without a spec, got a %s reply (spec_ready=%t)", r.Type, r.SpecReady)
		}
	case "spec":
		if asked || !r.SpecReady {
			return fmt.Errorf("expected a finished spec, got a %s reply (spec_ready=%t)", r.Type, r.SpecReady)
		}
	default:
		return fmt.Errorf("case expects %q: use ask, answer or spec", expect)
	}
	return nil
}

// CheckCorrection grades the reply to an owner's correction (Communication rule 8): the spec and a
// memory line both state every `keep` text, and neither carries a `drop` text (a non-goal may name
// the old value). Matching is case-insensitive on the decoded text, not on the JSON, so escapes and
// key names do not matter. It is substring matching: a stale value reworded ("500-row files") is not
// seen, and a keep or drop text must be written so that a longer number does not contain it.
func CheckCorrection(raw []byte, keep, drop []string) error {
	var r struct {
		Spec   map[string]json.RawMessage `json:"spec"`
		Memory []struct {
			Text string `json:"text"`
		} `json:"memory"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("correction: %w", err)
	}
	if len(r.Spec) == 0 {
		return errors.New("correction: the reply carries no spec to update")
	}
	delete(r.Spec, "non_goals") // a dropped value may be named there as out of scope
	var spec, mem strings.Builder
	for _, v := range r.Spec {
		collectStrings(v, &spec)
	}
	for _, m := range r.Memory {
		mem.WriteString(strings.ToLower(m.Text) + "\n")
	}
	for _, k := range keep {
		k = strings.ToLower(k)
		if !strings.Contains(spec.String(), k) {
			return fmt.Errorf("correction: the spec lost %q", k)
		}
		if !strings.Contains(mem.String(), k) {
			return fmt.Errorf("correction: the memory lost %q", k)
		}
	}
	for _, d := range drop {
		d = strings.ToLower(d)
		if strings.Contains(spec.String(), d) {
			return fmt.Errorf("correction: the spec still carries the old %q", d)
		}
		if strings.Contains(mem.String(), d) {
			return fmt.Errorf("correction: the memory still carries the old %q", d)
		}
	}
	return nil
}

// collectStrings writes every string value of a JSON document, lower-cased, one per line.
func collectStrings(raw json.RawMessage, b *strings.Builder) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case string:
			b.WriteString(strings.ToLower(t) + "\n")
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
}
