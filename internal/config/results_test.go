package config

import (
	"io/fs"
	"os"
	"testing"
)

var resultKinds = []Kind{
	KindLeadReply, KindSpec, KindBuilderResult, KindVerdict,
	KindFinding, KindNote, KindRubricResult, KindEvalResult,
}

// TestResultExamplesAreValid checks testdata/results/<kind>.json for every
// role result schema.
func TestResultExamplesAreValid(t *testing.T) {
	t.Parallel()
	root := os.DirFS("../../testdata/results")
	v := NewValidator()
	for _, kind := range resultKinds {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			doc, err := fs.ReadFile(root, string(kind)+".json")
			if err != nil {
				t.Fatal(err)
			}
			if err := v.ValidateJSON(kind, doc); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestInvalidResultsAreRejected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		kind Kind
		doc  string
	}{
		{"return needs findings", KindVerdict, `{"verdict":"return","summary":"s"}`},
		{"escalate needs a reason", KindVerdict, `{"verdict":"escalate","summary":"s"}`},
		{"agents cannot lower blast radius", KindVerdict, `{"verdict":"pass","summary":"s","raise_blast_radius":{"level":"low","reason":"r"}}`},
		{"nested finding is checked", KindVerdict, `{"verdict":"return","summary":"s","findings":[{"rule_id":"x"}]}`},
		{"blocked needs a reason", KindBuilderResult, `{"status":"blocked","summary":"s","changed_files":[]}`},
		{"question needs options", KindLeadReply, `{"type":"question","understood":"The owner wants the change described in the task.","text":"t"}`},
		{"criteria need Given/When/Then", KindSpec, `{"title":"t","goal":"g","prd_path":"p","acceptance_criteria":[{"id":"AC-1","given":"g"}],"tasks":[{"id":"T-1","title":"t"}]}`},
		{"fail needs findings", KindRubricResult, `{"rubric_id":"r","answer":"fail"}`},
		{"unknown layer", KindFinding, `{"rule_id":"r","category":"code","severity":"block","layer_caught":"L9","message":"m"}`},
		{"cache key is sha256", KindEvalResult, `{"task_id":"t","set":"fast","level":"E1","candidate":"c","passed":true,"cache_key":"abc"}`},
		{"not json", KindFinding, `{`},
	}
	v := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := v.ValidateJSON(tt.kind, []byte(tt.doc)); err == nil {
				t.Errorf("ValidateJSON accepted an invalid %s", tt.kind)
			}
		})
	}
}
