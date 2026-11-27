package runner

import (
	"reflect"
	"testing"

	"github.com/ilyachakun/nightshift/internal/assurance/check"
	"github.com/ilyachakun/nightshift/internal/customization/stack"
)

// TestStackToolCoversCheckTool keeps the copy in check.go complete: every field of check.Tool
// must exist in stack.Tool with the same type, so a new check field cannot be left unset.
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
