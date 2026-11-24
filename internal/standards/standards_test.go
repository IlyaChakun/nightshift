package standards_test

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/standards"
)

func TestChecklistPicksActiveStandardsOfTheStacks(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"industry/go/style.yaml":    {Data: []byte("id: go.style\ntitle: Go style\nstatus: active\n")},
		"industry/go/style.md":      {Data: []byte("# Go style\n")},
		"industry/go/lint.yaml":     {Data: []byte("id: go.lint\ntitle: Lint\nstatus: draft\n")},
		"industry/python/ruff.yaml": {Data: []byte("id: py.ruff\ntitle: Ruff\nstatus: active\n")},
		"industry/docs/adr.yaml":    {Data: []byte("id: docs.adr\ntitle: ADR\nstatus: active\n")},
		"industry/go/notes.txt":     {Data: []byte("ignored")},
	}
	got, err := standards.Checklist(fsys, []string{"go", "docs"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ADR (docs.adr)", "Go style (go.style), details: standards/industry/go/style.md"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := standards.Checklist(fstest.MapFS{"a/go/x.yaml": {Data: []byte(": :")}}, []string{"go"}); err == nil {
		t.Fatal("a broken standard was accepted")
	}
}
