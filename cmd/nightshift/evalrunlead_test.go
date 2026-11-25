package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEvalRunLeadGradesRepliesByTheChatRules replays testdata/eval/lead-eval.yaml: two
// replies follow the rules and one is too long, so the run reports 2 of 3 and fails.
func TestEvalRunLeadGradesRepliesByTheChatRules(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := evalRun([]string{"--role", "lead", "--harness", "fake", "--scenario", "../../testdata/eval/lead-eval.yaml",
		"--cases", "../../evals/lead/cases.yaml", "--roles", "../../roles", "--skills", "../../skills", "--out", out, "--candidate", "t"}, &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit %d, stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "lead: 2 of 3 cases") || !strings.Contains(stdout.String(), "long-answer fail") {
		t.Fatalf("stdout:\n%s", stdout.String())
	}
	raw, err := os.ReadFile(filepath.Join(out, "lead", "t", "long-answer.json"))
	if err != nil || !strings.Contains(string(raw), "61 words") {
		t.Fatalf("result file: %v %s", err, raw)
	}
}
