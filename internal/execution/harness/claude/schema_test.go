package claude

import (
	"encoding/json"
	"testing"

	"github.com/ilyachakun/nightshift/schemas"
)

// The first live Lead run got "input_schema does not support oneOf, allOf, or anyOf at the
// top level" for lead-reply.json.
func TestToolSchemaHasNoTopLevelCombinators(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"builder-result.json", "verdict.json", "lead-reply.json"} {
		raw, err := schemas.FS().ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		out, err := toolSchema(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, k := range topLevel {
			if _, ok := doc[k]; ok {
				t.Errorf("%s keeps %q at the top", name, k)
			}
		}
		if doc["type"] != "object" || doc["properties"] == nil {
			t.Errorf("%s lost its shape", name)
		}
	}
}
