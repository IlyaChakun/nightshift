package codex

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/schemas"
)

// OpenAI answered "'schema.if' is not permitted" to builder-result.json on the first live run.
func TestStrictResultSchemas(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"builder-result.json", "verdict.json"} {
		raw, err := schemas.FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		out, err := strictSchema(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, bad := range unsupported {
			if strings.Contains(string(out), `"`+bad+`":`) {
				t.Errorf("%s still holds %s", name, bad)
			}
		}
	}
}

// A strict answer writes null for what it left out; the original schema must accept the result.
func TestStrictAnswerMatchesTheOriginalSchema(t *testing.T) {
	t.Parallel()
	raw, err := schemas.FS.ReadFile("builder-result.json")
	if err != nil {
		t.Fatal(err)
	}
	answer := `{"status":"done","summary":"s","changed_files":["a"],"tests":{"added":null,"runs":[]},"disputes":null,"notes":null,"blocked_reason":null}`
	if err := harness.ValidateOutput(raw, withoutNulls([]byte(answer))); err != nil {
		t.Error(err)
	}
}
