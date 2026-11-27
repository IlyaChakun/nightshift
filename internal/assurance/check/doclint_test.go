package check

import "testing"

func TestDocLint(t *testing.T) {
	adr := DocRules{Sections: []string{"Context and Problem Statement", "Considered Options", "Decision Outcome"}, Front: true, MaxLines: 12}
	tests := []struct {
		name, text string
		rules      DocRules
		want       []string
	}{
		{"complete", "---\nstatus: x\n---\n# T\n## Context and Problem Statement\n## Considered Options\n## Decision Outcome\n", adr, nil},
		{"no front, no sections", "# T\n## Considered Options\n", adr, []string{
			"line 1: the document must start with a front matter block",
			`missing section "Context and Problem Statement"`,
			`missing section "Decision Outcome"`,
		}},
		{"too long", "a\nb\nc\n", DocRules{MaxLines: 2}, []string{"the document has 3 lines, the limit is 2"}},
		{"open fence", "# T\n```go\ncode\n", DocRules{}, []string{"line 2: this code fence is never closed"}},
		{"heading inside a fence does not count", "```\n## Decision Outcome\n```\n", DocRules{Sections: []string{"Decision Outcome"}}, []string{`missing section "Decision Outcome"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DocLint(tt.text, tt.rules)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i, v := range got {
				if v.String() != tt.want[i] {
					t.Errorf("violation %d = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}
