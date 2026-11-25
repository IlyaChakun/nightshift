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

// LeadReply is a validated Lead chat message.
type LeadReply struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Recommended *int   `json:"recommended"`
	Options     []struct {
		Label string `json:"label"`
	} `json:"options"`
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
