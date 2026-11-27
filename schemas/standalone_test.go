package schemas_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ilyachakun/nightshift/schemas"
)

// The claude CLI refused builder-result.json on the first real run: it knows neither the
// 2020-12 meta-schema nor sibling files.
func TestStandaloneResultSchemas(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"builder-result.json", "verdict.json"} {
		raw, err := schemas.FS().ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		out, err := schemas.Standalone(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, bad := range []string{`"$schema"`, `"$id"`, `.json"`} {
			if strings.Contains(string(out), bad) {
				t.Errorf("%s still holds %s", name, bad)
			}
		}
		if !strings.Contains(string(out), `"gotcha"`) {
			t.Errorf("%s lost the note schema", name)
		}
	}
}

func TestStandaloneRejectsUnknownReference(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{"$ref":"missing.json"}`, `not json`} {
		if _, err := schemas.Standalone([]byte(raw)); err == nil {
			t.Errorf("%s: want an error", raw)
		}
	}
}

// Every schema a role answers with must compile on its own: the first live Lead run failed on a
// reference a sibling file kept to its own definitions.
func TestEveryResultSchemaCompilesStandalone(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"builder-result.json", "verdict.json", "lead-reply.json", "rubric-result.json", "eval-result.json"} {
		raw, err := schemas.FS().ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		out, err := schemas.Standalone(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("file:///standalone.json", doc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := c.Compile("file:///standalone.json"); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
