package policy_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ilyachakun/nightshift/internal/assurance/policy"
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

func TestProjectGuardProtectsCommandScripts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"tools/setup.sh", true}, {"scripts/verify", true},
		{"tests/unit/test_a.py", false}, {"src/main.py", false}, {"package.json", false},
	}
	wrapped := policy.ProjectGuard(func(context.Context, string, string, string) (string, bool, error) {
		return "verify: env FOO=1 sh -c ./scripts/check\nsetup: time ./bin/prepare\n", true, nil
	})
	for _, path := range []string{"scripts/check", "bin/prepare"} {
		if err := wrapped(t.Context(), "m", "base", []string{path}); err == nil {
			t.Errorf("%s runs behind a wrapper and was not protected", path)
		}
	}
	// A schema-valid project file with an object in tests.external must still parse.
	object := policy.ProjectGuard(func(context.Context, string, string, string) (string, bool, error) {
		return "verify: make verify\ntests: { ci: github-actions, external: { kind: manual, ask: approvers } }\n", true, nil
	})
	if err := object(t.Context(), "m", "base", []string{"src/a.go"}); err != nil {
		t.Errorf("a schema-valid project.yaml rejected an ordinary patch: %v", err)
	}
	guard := policy.ProjectGuard(func(context.Context, string, string, string) (string, bool, error) {
		return "setup: bash tools/setup.sh && npm ci\nverify: ./scripts/verify && pytest tests/unit\ntests: { ci: github-actions, external: null }\n", true, nil
	})
	for _, tt := range tests {
		err := guard(t.Context(), "m", "base", []string{tt.path})
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.path, err, tt.wantErr)
		}
	}
}

const deployProject = `verify: make verify
capabilities:
  merge_train: true
  preview: { kind: script, up: "./tools/preview-up.sh", down: "bash tools/preview-down.sh" }
  smoke: "sh tools/smoke.sh --quick"
  release: { kind: compose-local, file: deploy/release.yml }
  rollback: tools/rollback
`

// What a deployer or the toolchain would run is guarded from the Builder like verify and setup, and
// the compose recipes are privileged for the blast radius.
func TestProjectGuardProtectsDeployCommandScripts(t *testing.T) {
	t.Parallel()
	guard := policy.ProjectGuard(func(context.Context, string, string, string) (string, bool, error) {
		return deployProject, true, nil
	})
	for _, path := range []string{"tools/preview-up.sh", "tools/preview-down.sh", "tools/smoke.sh"} {
		if err := guard(t.Context(), "m", "base", []string{path}); err == nil {
			t.Errorf("%s is run by a deploy capability and was not protected", path)
		}
	}
	if err := guard(t.Context(), "m", "base", []string{"src/app.go", "tests/smoke_test.go"}); err != nil {
		t.Errorf("an ordinary patch was rejected: %v", err)
	}
}

func TestProjectExecuted(t *testing.T) {
	t.Parallel()
	got, err := policy.ProjectExecuted(deployProject)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tools/preview-up.sh", "tools/preview-down.sh", "tools/smoke.sh", "tools/rollback", "deploy/release.yml"} {
		if !slices.Contains(got, want) {
			t.Errorf("%s is missing from %v", want, got)
		}
	}
	if _, err := policy.ProjectExecuted("verify: [unclosed"); err == nil {
		t.Error("an unreadable project file was accepted")
	}
	none, err := policy.ProjectExecuted("verify: go test ./...\n")
	if err != nil || len(none) != 0 {
		t.Errorf("plain commands name no script: %v, %v", none, err)
	}
}

// Odd but schema-near shapes must neither panic nor drop what is clear: a dotted name is a name,
// a preview that is not an object names nothing, `./` prefixes are removed.
func TestProjectExecutedOddShapes(t *testing.T) {
	t.Parallel()
	got, err := policy.ProjectExecuted("capabilities:\n  preview: null\n  smoke: { kind: compose-local, file: ./deploy/smoke..yml }\n  release: [a, b]\n  rollback: { up: 3, down: [x] }\n")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"deploy/smoke..yml"}) {
		t.Errorf("got %v", got)
	}
	if _, err := policy.ProjectExecuted("capabilities:\n  smoke: { kind: compose-local, file: ../x.yml }\n"); err != nil {
		t.Errorf("an escaping recipe path is ignored here (the schema refuses it), got %v", err)
	}
}
