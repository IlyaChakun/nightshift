package main

import (
	"bytes"
	"testing"

	"github.com/ilyachakun/nightshift/internal/assurance/rubric"
	"github.com/ilyachakun/nightshift/internal/execution/harness/claude"
)

func TestCheckRubricFlagsGoTogether(t *testing.T) {
	for _, args := range [][]string{
		{"check", "--out", "x", "--standards", "standards"},
		{"check", "--out", "x", "--judge-auth", "api_key"},
		{"check", "--out", "x", "--standards", "standards", "--judge-auth", "bogus"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitUsage {
			t.Errorf("%v: code %d, %s", args, code, stderr.String())
		}
	}
}

func TestTheRubricJudgeGetsItsCredentialAndNoTools(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "k")
	t.Setenv("GITHUB_TOKEN", "must-not-pass")
	j, _, err := rubricJudge("standards", "api_key")
	if err != nil {
		t.Fatal(err)
	}
	hj := j.(rubric.HarnessJudge)
	if hj.Env["ANTHROPIC_API_KEY"] != "k" || hj.Env["GITHUB_TOKEN"] != "" {
		t.Fatalf("env %v", hj.Env)
	}
	if len(hj.Harness.(*claude.Harness).DisallowedTools) == 0 {
		t.Fatal("the judge turn must have no tools")
	}
}
