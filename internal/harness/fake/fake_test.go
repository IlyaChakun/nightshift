package fake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/testutil/golden"
	"github.com/ilyachakun/nightshift/schemas"
)

const happyPath = "../../../testdata/scenarios/happy-path.yaml"

func schema(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := fs.ReadFile(schemas.FS, name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestReplayHappyPath replays every agent step through the validating
// wrapper and compares the outputs with a golden file.
func TestReplayHappyPath(t *testing.T) {
	t.Parallel()
	f, err := Load(happyPath)
	if err != nil {
		t.Fatal(err)
	}
	h := harness.Validating{Next: f}
	workspace := t.TempDir()
	turns := []struct{ role, schema string }{
		{"lead", "spec.json"},
		{"builder", "builder-result.json"},
		{"verifier", "verdict.json"},
	}
	var log bytes.Buffer
	for _, turn := range turns {
		res, err := h.Run(t.Context(), harness.RunRequest{
			Role:         harness.RoleConfig{Name: turn.role},
			Workspace:    workspace,
			OutputSchema: schema(t, turn.schema),
		})
		if err != nil {
			t.Fatalf("%s: %v", turn.role, err)
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, res.Output); err != nil {
			t.Fatal(err)
		}
		log.WriteString(turn.role + " " + res.SessionID + " " + compact.String() + "\n")
	}
	golden.Assert(t, "../../../testdata/golden/fake-happy-path.txt", log.Bytes())

	if _, err := os.Stat(filepath.Join(workspace, PatchFile)); err != nil {
		t.Errorf("builder patch not written: %v", err)
	}
	if n := f.Remaining(); n != 0 {
		t.Errorf("Remaining() = %d, want 0", n)
	}
	if _, err := f.Run(t.Context(), harness.RunRequest{Role: harness.RoleConfig{Name: "lead"}}); !errors.Is(err, ErrNoMoreSteps) {
		t.Errorf("extra run: err = %v, want ErrNoMoreSteps", err)
	}
}

func TestWrongRoleDoesNotConsumeStep(t *testing.T) {
	t.Parallel()
	f, err := Load(happyPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Run(t.Context(), harness.RunRequest{Role: harness.RoleConfig{Name: "builder"}})
	if !errors.Is(err, ErrWrongRole) {
		t.Fatalf("err = %v, want ErrWrongRole", err)
	}
	if n := f.Remaining(); n != 3 {
		t.Errorf("Remaining() = %d, want 3", n)
	}
}

func TestCanceledContext(t *testing.T) {
	t.Parallel()
	f, err := Load(happyPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.Run(ctx, harness.RunRequest{Role: harness.RoleConfig{Name: "lead"}}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("steps: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(dir, "missing.yaml"), bad} {
		if _, err := Load(path); err == nil {
			t.Errorf("Load(%s) succeeded, want an error", filepath.Base(path))
		}
	}
}
