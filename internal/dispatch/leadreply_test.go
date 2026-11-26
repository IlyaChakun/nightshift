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
		{"decision", `{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick one.","options":[{"label":"A"},{"label":"B"}],"recommended":1,"basis":{"checked":["docs/design.md section 7"],"confidence":"high"}}`, ""},
		{"question without understood", `{"type":"question","text":"Which?","options":[{"label":"A"},{"label":"B"}],"recommended":0,"basis":{"checked":["docs/design.md section 7"],"confidence":"high"}}`, "carries `understood`"},
		{"understood too long", `{"type":"blocker_or_summary","text":"x","understood":"` + strings.Repeat("word ", 26) + `"}`, "`understood` is 26 words"},
		{"understood of 24 long words", `{"type":"blocker_or_summary","text":"x","understood":"The owner wants nightly reporting exports converted automatically into downloadable spreadsheets, including archived historical records, filtered columns, localized currency formatting, timezone conversions, consistent pagination."}`, ""},
		{"too long", `{"type":"blocker_or_summary","text":"` + long + `"}`, "61 words"},
		{"decision without options", `{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick one."}`, "needs two or three options"},
		{"recommendation out of range", `{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick.","options":[{"label":"A"},{"label":"B"}],"recommended":2,"basis":{"checked":["docs/design.md section 7"],"confidence":"high"}}`, "points at no option"},
		{"summary with options", `{"type":"blocker_or_summary","text":"x","options":[{"label":"A"},{"label":"B"}]}`, "carries no options"},
		{"recommendation without basis", `{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick.","options":[{"label":"A"},{"label":"B"}],"recommended":0}`, "needs a basis"},
		{"nothing checked", `{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick.","options":[{"label":"A"},{"label":"B"}],"recommended":0,"basis":{"checked":[],"confidence":"medium"}}`, "names nothing"},
		{"low confidence", `{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick.","options":[{"label":"A"},{"label":"B"}],"recommended":0,"basis":{"checked":["x"],"confidence":"low"}}`, "confidence is low"},
		{"low confidence without a recommendation is honest", `{"type":"blocker_or_summary","text":"I could not open the vendor docs.","basis":{"checked":["roles/lead.md"],"inferred":["the API shape"],"confidence":"low"}}`, ""},
		{"low confidence with a spec", `{"type":"blocker_or_summary","text":"Ready.","spec_ready":true,"understood":"The owner wants the change described in the task.","basis":{"checked":["x"],"confidence":"low"},"spec":{"title":"CSV export","goal":"Export.","prd_path":"docs/prd/csv.md","acceptance_criteria":[{"id":"AC-1","given":"a","when":"b","then":"c"}],"tasks":[{"id":"T-1","title":"Add"}]}}`, "confidence is low"},
		{"basis too long", `{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick.","options":[{"label":"A"},{"label":"B"}],"recommended":0,"basis":{"checked":["` + strings.Repeat("a b ", 13) + `"],"confidence":"high"}}`, "the limit is 25"},
		{"spec without basis", `{"type":"blocker_or_summary","text":"Ready.","spec_ready":true,"understood":"The owner wants the change described in the task.","spec":{"title":"CSV export","goal":"Export.","prd_path":"docs/prd/csv.md","acceptance_criteria":[{"id":"AC-1","given":"a","when":"b","then":"c"}],"tasks":[{"id":"T-1","title":"Add"}]}}`, "needs a basis"},
		{"basis without recommendation is checked too", `{"type":"blocker_or_summary","text":"x","basis":{"checked":[],"confidence":"high"}}`, "names nothing"},
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

// TestCheckExpectGradesAskVersusSpec: a vague case must get a question, a clear one a spec, and
// a typo in the case is an error, not a case nobody can win.
func TestCheckExpectGradesAskVersusSpec(t *testing.T) {
	ask := LeadReply{Type: "question"}
	spec := LeadReply{Type: "blocker_or_summary", SpecReady: true}
	for name, tc := range map[string]struct {
		expect  string
		r       LeadReply
		wantErr bool
	}{
		"no expectation":      {"", spec, false},
		"ask, asked":          {"ask", ask, false},
		"ask, got a spec":     {"ask", spec, true},
		"ask, got a summary":  {"ask", LeadReply{Type: "blocker_or_summary"}, true},
		"spec, got a spec":    {"spec", spec, false},
		"spec, asked instead": {"spec", ask, true},
		"typo":                {"specs", spec, true},
	} {
		if err := CheckExpect(tc.expect, tc.r); (err != nil) != tc.wantErr {
			t.Errorf("%s: %v", name, err)
		}
	}
}
