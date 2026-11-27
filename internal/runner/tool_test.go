package runner

import (
	"reflect"
	"testing"

	"github.com/ilyachakun/nightshift/internal/assurance/check"
	"github.com/ilyachakun/nightshift/internal/customization/stack"
)

// TestStackToolCoversCheckTool keeps check.Tool buildable from stack.Tool: every field of
// check.Tool must exist in stack.Tool with the same type. A field added to both still needs a
// line in the copy in check.go.
func TestStackToolCoversCheckTool(t *testing.T) {
	t.Parallel()
	want, have := reflect.TypeFor[check.Tool](), reflect.TypeFor[stack.Tool]()
	for f := range want.Fields() {
		g, ok := have.FieldByName(f.Name)
		if !ok || g.Type != f.Type {
			t.Errorf("stack.Tool lacks field %s %s that check.Tool has", f.Name, f.Type)
		}
	}
}
