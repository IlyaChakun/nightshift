package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalRunSolvesATaskInAThrowawayRepo(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{"eval", "run", "--tasks", "../../evals/tasks", "--scenario", "../../testdata/evals/greeting.yaml", "--out", out, "--candidate", "fake-pass"}, &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "greeting pass") || !strings.Contains(stdout.String(), "solved 1 of 1") {
		t.Fatalf("code %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	raw, err := os.ReadFile(filepath.Join(out, "fake-pass", "greeting.json")) //nolint:gosec // G304: a path made by this test
	if err != nil || !strings.Contains(string(raw), `"passed": true`) {
		t.Fatalf("result file: %v\n%s", err, raw)
	}
}

func TestEvalRunNeedsAScenarioAndAPlainCandidate(t *testing.T) {
	for _, args := range [][]string{
		{"eval", "run"},
		{"eval", "run", "--scenario", "x", "--candidate", "../x"},
		{"eval", "run", "--harness", "claude"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitUsage {
			t.Errorf("%v: code %d, %s", args, code, stderr.String())
		}
	}
}

func TestEvalRunFailsWhenTheSetIsEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"eval", "run", "--tasks", "../../evals/tasks", "--scenario", "../../testdata/evals/greeting.yaml", "--set", "holdout", "--out", t.TempDir()}
	if code := run(args, &stdout, &stderr); code != exitFailure {
		t.Fatalf("code %d, want failure for an empty set", code)
	}
}
