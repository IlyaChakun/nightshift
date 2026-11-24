package schemas_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ilyachakun/nightshift/schemas"
)

func TestAddToCompilesEverySchema(t *testing.T) {
	t.Parallel()
	c := jsonschema.NewCompiler()
	if err := schemas.AddTo(c); err != nil {
		t.Fatal(err)
	}
	names, err := fs.Glob(schemas.FS, "*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no embedded schemas")
	}
	for _, name := range names {
		if _, err := c.Compile(schemas.Base + name); err != nil {
			t.Errorf("compile %s: %v", strings.TrimSuffix(name, ".json"), err)
		}
	}
}
