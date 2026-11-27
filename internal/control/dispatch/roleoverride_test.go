package dispatch

import (
	"strings"
	"testing"
)

func TestParseRoleOverride(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, doc, wantErr string
	}{
		{"empty", "", ""},
		{"both", "instructions: [docs/lead.md]\nskills: [own/audit]\n", ""},
		{"unknown key", "output_schema: schemas/x.json\n", "output_schema"},
		{"limits cannot be changed", "limits: { minutes: 999, tokens: 1 }\n", "limits"},
		{"absolute path", "instructions: [/etc/passwd]\n", "bad instruction path"},
		{"traversal", "instructions: [../x.md]\n", "bad instruction path"},
		{"unclean path", "instructions: [docs/../x.md]\n", "bad instruction path"},
		{"option-like path", "instructions: [-rf]\n", "bad instruction path"},
		{"bad skill id", "skills: ['Own/Audit']\n", "bad skill id"},
		{"skill without set", "skills: [audit]\n", "bad skill id"},
		{"too many", "skills: [a/a, a/b, a/c, a/d, a/e, a/f, a/g, a/h, a/i]\n", "more than 8"},
	} {
		_, err := parseRoleOverride(tc.doc)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: error %v, want %q", tc.name, err, tc.wantErr)
		}
	}
}
