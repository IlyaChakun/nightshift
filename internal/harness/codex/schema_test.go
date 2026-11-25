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
	if err := harness.ValidateOutput(raw, withoutNulls([]byte(answer), raw)); err != nil {
		t.Error(err)
	}
}

// A null the schema allows is the model's answer, not strict mode's filler.
func TestNullsTheSchemaAllowsStay(t *testing.T) {
	t.Parallel()
	schema := []byte(`{"type":"object","required":["pick"],"properties":{"pick":{"type":["integer","null"]},"why":{"type":"string"}}}`)
	got := string(withoutNulls([]byte(`{"pick":null,"why":null}`), schema))
	if got != `{"pick":null}` {
		t.Fatalf("got %s", got)
	}
}

// Where the schema is silent (a union), strict mode's null filler still goes.
func TestNullsUnderAUnionAreFiller(t *testing.T) {
	t.Parallel()
	schema := []byte(`{"type":"object","properties":{"x":{"anyOf":[{"type":"object","properties":{"a":{"type":"string"}}}]}}}`)
	got := string(withoutNulls([]byte(`{"x":{"a":null}}`), schema))
	if got != `{"x":{}}` {
		t.Fatalf("got %s", got)
	}
}
