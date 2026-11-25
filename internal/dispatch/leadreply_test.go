package dispatch

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
)

func TestCheckLeadReply(t *testing.T) {
	v := config.NewValidator()
	long := strings.Repeat("word ", 61)
	tests := []struct {
		name, raw, wantErr string
	}{
		{"summary", `{"type":"blocker_or_summary","text":"Done, the PR is up."}`, ""},
		{"decision", `{"type":"decision_needed","text":"Pick one.","options":[{"label":"A"},{"label":"B"}],"recommended":1}`, ""},
		{"too long", `{"type":"blocker_or_summary","text":"` + long + `"}`, "61 words"},
		{"decision without options", `{"type":"decision_needed","text":"Pick one."}`, "needs two or three options"},
		{"recommendation out of range", `{"type":"decision_needed","text":"Pick.","options":[{"label":"A"},{"label":"B"}],"recommended":2}`, "points at no option"},
		{"summary with options", `{"type":"blocker_or_summary","text":"x","options":[{"label":"A"},{"label":"B"}]}`, "carries no options"},
		{"not in the schema", `{"type":"chitchat","text":"hi"}`, "lead reply"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CheckLeadReply(v, []byte(tt.raw))
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
