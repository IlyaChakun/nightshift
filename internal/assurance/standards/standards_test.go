package standards_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/assurance/standards"
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

func TestListAndSync(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"industry/go/lint.yaml":               {Data: []byte("id: go.lint\ntitle: Lint\nenforcement: tool\nseverity: block\nstatus: active\n")},
		"industry/go/files/.golangci.yml":     {Data: []byte("version: \"2\"\n")},
		"industry/python/files/ruff.toml":     {Data: []byte("line-length = 100\n")},
		"industry/python/files/sub/extra.cfg": {Data: []byte("x\n")},
	}
	cards, err := standards.List(fsys, []string{"go"})
	if err != nil || len(cards) != 1 || cards[0].ID != "go.lint" || cards[0].Stack != "go" || cards[0].Severity != "block" {
		t.Fatalf("cards %+v %v", cards, err)
	}
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, ".golangci.yml"), []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	written, kept, err := standards.Sync(fsys, []string{"go", "python"}, dest)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(written, []string{"ruff.toml", "sub/extra.cfg"}) || !slices.Equal(kept, []string{".golangci.yml"}) {
		t.Fatalf("written %v kept %v", written, kept)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, ".golangci.yml")); string(b) != "mine\n" { //nolint:gosec // G304: a path inside the test's temporary directory
		t.Fatalf("the project's own config was replaced: %q", b)
	}
}
