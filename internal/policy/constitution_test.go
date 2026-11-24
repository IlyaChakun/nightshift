package policy_test

import (
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
