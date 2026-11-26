package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/harness"
)

// TestEvalRunLeadGradesRepliesByTheChatRules replays testdata/eval/lead-eval.yaml: two
// replies follow the rules and one is too long, so the run reports 2 of 3 and fails.
func TestEvalRunLeadGradesRepliesByTheChatRules(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := evalRun([]string{"--role", "lead", "--harness", "fake", "--scenario", "../../testdata/eval/lead-eval.yaml",
		"--cases", "../../testdata/eval/lead-cases.yaml", "--roles", "../../roles", "--skills", "../../skills", "--fixture", "../../evals/lead/repo", "--out", out, "--candidate", "t"}, &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit %d, stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "lead: 2 of 3 cases") || !strings.Contains(stdout.String(), "long-answer fail") {
		t.Fatalf("stdout:\n%s", stdout.String())
	}
	raw, err := os.ReadFile(filepath.Join(out, "lead", "t", "long-answer.json")) //nolint:gosec // G304: a path under t.TempDir()
	if err != nil || !strings.Contains(string(raw), "61 words") {
		t.Fatalf("result file: %v %s", err, raw)
	}
}

// TestEvalRunLeadRejectsUnsafeCaseIDs: a case id becomes a file name, so a path or a repeat stops the run.
func TestEvalRunLeadRejectsUnsafeCaseIDs(t *testing.T) {
	for name, ids := range map[string]string{"path": "- {id: ../x, owner: hi, must: [a]}", "repeat": "- {id: a, owner: hi, must: [a]}\n- {id: a, owner: hi, must: [a]}"} {
		t.Run(name, func(t *testing.T) {
			cases := filepath.Join(t.TempDir(), "cases.yaml")
			if err := os.WriteFile(cases, []byte(ids), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := evalRun([]string{"--role", "lead", "--harness", "fake", "--scenario", "../../testdata/eval/lead-eval.yaml",
				"--cases", cases, "--roles", "../../roles", "--skills", "../../skills", "--fixture", "../../evals/lead/repo", "--out", t.TempDir()}, &stdout, &stderr)
			if code != exitFailure || !strings.Contains(stderr.String(), "not a plain name") {
				t.Fatalf("exit %d, stderr %s", code, stderr.String())
			}
		})
	}
}

// stubHarness records the request it gets and answers with a fixed result.
type stubHarness struct {
	res harness.RunResult
	err error
	req harness.RunRequest
	// sawFixture is set when the workspace held the fixture's AGENTS.md during the run.
	sawFixture bool
}

func (s *stubHarness) Run(_ context.Context, req harness.RunRequest) (harness.RunResult, error) {
	s.req = req
	if _, err := os.Stat(filepath.Join(req.Workspace, "AGENTS.md")); err == nil {
		s.sawFixture = true
	}
	return s.res, s.err
}

func leadCasesFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cases.yaml")
	if err := os.WriteFile(p, []byte("- {id: a, owner: hi, must: [x]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestEvalRunLeadHandsTheHarnessItsCredential: the environment of the options reaches the
// harness request, and nothing of it is written to the output or the result file.
func TestEvalRunLeadHandsTheHarnessItsCredential(t *testing.T) {
	good := `{"type":"blocker_or_summary","text":"I cannot check that."}`
	h := &stubHarness{res: harness.RunResult{Output: json.RawMessage(good), ExitReason: harness.ExitDone}}
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := evalRunLead(t.Context(), h, leadEvalOptions{
		Cases: leadCasesFile(t), RolesDir: "../../roles", SkillsDir: "../../skills", Fixture: "../../evals/lead/repo", Out: out,
		Env: map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "sk-test-secret"},
	}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d, stderr %s", code, stderr.String())
	}
	if !h.sawFixture {
		t.Fatal("the case did not run in a copy of the fixture repository")
	}
	if h.req.Env["CLAUDE_CODE_OAUTH_TOKEN"] != "sk-test-secret" {
		t.Fatalf("the harness got env %v", h.req.Env)
	}
	files, _ := filepath.Glob(filepath.Join(out, "*"))
	for _, f := range files {
		if raw, _ := os.ReadFile(f); strings.Contains(string(raw), "sk-test-secret") { //nolint:gosec // G304: a file under t.TempDir()
			t.Fatalf("the credential is in %s", f)
		}
	}
	if strings.Contains(stdout.String()+stderr.String(), "sk-test-secret") {
		t.Fatal("the credential is in the output")
	}
}

// TestEvalRunLeadReportsAFailedLoginAsNotRun: a login failure, whether the harness returns an
// error or an error exit, is "eval did not run", never a graded case and no result file.
func TestEvalRunLeadReportsAFailedLoginAsNotRun(t *testing.T) {
	login := `{"error":"Not logged in · Please run /login"}`
	for name, h := range map[string]*stubHarness{
		"error exit":     {res: harness.RunResult{Output: json.RawMessage(login), ExitReason: harness.ExitError}},
		"returned error": {err: errors.New("claude: exit status 1: Not logged in")},
	} {
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			var stdout, stderr bytes.Buffer
			code := evalRunLead(t.Context(), h, leadEvalOptions{Cases: leadCasesFile(t), RolesDir: "../../roles", SkillsDir: "../../skills", Out: out}, &stdout, &stderr)
			if code != exitFailure || !strings.Contains(stderr.String(), "eval did not run") || strings.Contains(stdout.String(), "a fail") {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
			}
			if files, _ := filepath.Glob(filepath.Join(out, "*")); len(files) != 0 {
				t.Fatalf("a result file was written: %v", files)
			}
		})
	}
}

// TestEvalEnvReadsTheHarnessDirectoryLikeTheWorker: the credential comes from <secrets>/harness,
// and a missing one stops the run before any case.
func TestEvalEnvReadsTheHarnessDirectoryLikeTheWorker(t *testing.T) {
	secrets := t.TempDir()
	if _, err := evalEnv("claude", "subscription", secrets); err == nil || !strings.Contains(err.Error(), "CLAUDE_CODE_OAUTH_TOKEN") {
		t.Fatalf("a missing credential gave %v", err)
	}
	dir := filepath.Join(secrets, "harness")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE_CODE_OAUTH_TOKEN"), []byte("tok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTPS_PROXY", "http://proxy:3128")
	env, err := evalEnv("claude", "subscription", secrets)
	if err != nil || env["CLAUDE_CODE_OAUTH_TOKEN"] != "tok" || env["HTTPS_PROXY"] != "http://proxy:3128" {
		t.Fatalf("env %v, err %v", env, err)
	}
	if env, err := evalEnv("fake", "", secrets); err != nil || len(env) != 0 {
		t.Fatalf("the fake harness needs no credential: %v %v", env, err)
	}
}

// TestCheckExpectGradesAskVersusSpec: a vague case must get a question, a clear one a spec.
func TestCheckExpectGradesAskVersusSpec(t *testing.T) {
	ask := dispatch.LeadReply{Type: "question"}
	spec := dispatch.LeadReply{Type: "blocker_or_summary", SpecReady: true}
	for name, tc := range map[string]struct {
		expect  string
		r       dispatch.LeadReply
		wantErr bool
	}{
		"no expectation":      {"", spec, false},
		"ask, asked":          {"ask", ask, false},
		"ask, got a spec":     {"ask", spec, true},
		"spec, got a spec":    {"spec", spec, false},
		"spec, asked instead": {"spec", ask, true},
		"unknown expectation": {"maybe", ask, true},
	} {
		if err := checkExpect(tc.expect, tc.r); (err != nil) != tc.wantErr {
			t.Errorf("%s: %v", name, err)
		}
	}
}
