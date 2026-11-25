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

// LeadReply is a validated Lead chat message.
type LeadReply struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Recommended *int   `json:"recommended"`
	Options     []struct {
		Label string `json:"label"`
	} `json:"options"`
	Basis     *LeadBasis `json:"basis"`
	SpecReady bool       `json:"spec_ready"`
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
	if r.Basis.Confidence == "low" {
		return errors.New("lead reply: confidence is low; check the source before you recommend or write a spec")
	}
	if n := len(strings.Fields(basisLine(r.Basis))); n > maxBasisWords {
		return fmt.Errorf("lead reply: the basis is %d words, the limit is %d", n, maxBasisWords)
	}
	return nil
}

// basisLine renders the basis as one short line for the card.
func basisLine(b *LeadBasis) string {
	if b == nil {
		return ""
	}
	line := "Checked: " + strings.Join(b.Checked, "; ")
	if len(b.Inferred) > 0 {
		line += ". Inferred: " + strings.Join(b.Inferred, "; ")
	}
	return line + ". Confidence: " + b.Confidence
}
