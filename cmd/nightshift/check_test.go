package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const failingTool = `printf '%s' '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"x"}},"results":[{"ruleId":"R","level":"error","message":{"text":"bad line"}}]}]}' > "$NIGHTSHIFT_SARIF"`

func TestCheckCommand(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "out")
	var stdout, stderr bytes.Buffer
	args := []string{"check", "--dir", t.TempDir(), "--out", out, "--tool", "lint=" + failingTool}
	if code := run(args, &stdout, &stderr); code != exitFailure || !strings.Contains(stdout.String(), "block x.r: bad line") {
		t.Fatalf("new violation: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	// The findings of that run, used as the baseline, make the same run pass.
	args = append(args, "--baseline", filepath.Join(out, "findings.json"))
	stdout.Reset()
	if code := run(args, &stdout, &stderr); code != exitOK || stdout.Len() != 0 {
		t.Fatalf("baseline: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestCheckCommandUsage(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][]string{
		"no out":        {"check"},
		"bad tool":      {"check", "--out", t.TempDir(), "--tool", "nocommand"},
		"bad flag":      {"check", "--nope"},
		"bad baseline":  {"check", "--out", t.TempDir(), "--baseline", filepath.Join(t.TempDir(), "missing.json")},
		"corrupt input": {"check", "--out", t.TempDir(), "--baseline", corrupt(t)},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code == exitOK {
			t.Errorf("%s: exit code 0", name)
		}
	}
}

func corrupt(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "findings.json")
	if err := os.WriteFile(p, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
