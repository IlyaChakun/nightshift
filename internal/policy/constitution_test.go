package policy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ilyachakun/nightshift/internal/policy"
)

func TestConstitution(t *testing.T) {
	c, err := policy.NewConstitution([]string{"scripts/*.sh", "docs/adr/**"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		path      string
		protected bool
	}{
		{".github/workflows/ci.yml", true},
		{".github/copilot-instructions.md", true},
		{".nightshift/project.yaml", true},
		{".claude/settings.json", true},
		{".mcp.json", true},
		{"Makefile", true},
		{"makefile", true}, // a case-insensitive file system
		{"AGENTS.md", true},
		{"mise.lock", true},
		{"scripts/deploy.sh", true},
		{"docs/adr/0001-x.md", true},
		{"scripts/lib/x.sh", false}, // * stays within one directory
		{"internal/app/main.go", false},
		{"docs/Makefile.md", false},
		{"sub/Makefile", false}, // anchored at the root
		{"../outside", true},
		{"/abs/path", true},
		{"a/../.github/x", true}, // not clean
	} {
		err := c.Check([]string{"internal/ok.go", tt.path})
		if got := errors.Is(err, policy.ErrProtected); got != tt.protected {
			t.Errorf("%q: protected = %v, want %v (%v)", tt.path, got, tt.protected, err)
		}
	}
	if err := c.Check(nil); err != nil {
		t.Errorf("empty patch: %v", err)
	}
}

func TestConstitutionRejectsRootEscapes(t *testing.T) {
	for _, p := range []string{"", "/etc/**", "../x", "."} {
		if _, err := policy.NewConstitution([]string{p}); err == nil {
			t.Errorf("pattern %q accepted", p)
		}
	}
}

func TestProjectGuard(t *testing.T) {
	t.Parallel()
	showing := func(doc string, ok bool) func(context.Context, string, string, string) (string, bool, error) {
		return func(_ context.Context, _, _, name string) (string, bool, error) {
			if name != ".nightshift/project.yaml" {
				t.Errorf("read %q, want the project file", name)
			}
			return doc, ok, nil
		}
	}
	const own = "name: example-solo\nprotected: [docs/legal/**, \"*.lock\"]\n"
	tests := []struct {
		name    string
		show    func(context.Context, string, string, string) (string, bool, error)
		paths   []string
		wantErr bool
	}{
		{"project path is protected", showing(own, true), []string{"docs/legal/terms.md"}, true},
		{"project glob is protected", showing(own, true), []string{"yarn.lock"}, true},
		{"other path passes", showing(own, true), []string{"src/main.go"}, false},
		{"built-in still holds", showing(own, true), []string{"Makefile"}, true},
		{"no project file: built-in only", showing("", false), []string{"docs/legal/terms.md"}, false},
		{"broken yaml rejects", showing("protected: [", true), []string{"src/main.go"}, true},
		{"root escape pattern rejects", showing("protected: ['../x']", true), []string{"src/main.go"}, true},
		{"read error rejects", func(context.Context, string, string, string) (string, bool, error) {
			return "", false, errors.New("boom")
		}, []string{"src/main.go"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := policy.ProjectGuard(tt.show)(t.Context(), "m", "base", tt.paths)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
