package rubric

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
)

type reply struct {
	out  string
	exit harness.ExitReason
	req  harness.RunRequest
}

func (r *reply) Run(_ context.Context, req harness.RunRequest) (harness.RunResult, error) {
	r.req = req
	return harness.RunResult{Output: json.RawMessage(r.out), ExitReason: r.exit}, nil
}

func ask(t *testing.T, r *reply) (Answer, error) {
	t.Helper()
	req := Request{Card: Card{ID: "prd.x", Title: "T", Question: "Q?", Pass: "p", Fail: "f"}, Doc: Doc{Path: "d.md", Text: "body"}}
	return HarnessJudge{Harness: r}.Judge(t.Context(), req)
}

func TestTheHarnessJudgeAsksReadOnlyAndParsesTheAnswer(t *testing.T) {
	r := &reply{out: `{"answer":"fail","rationale":"no goal","evidence":["x"]}`}
	a, err := ask(t, r)
	if err != nil || a.Answer != Fail || a.Rationale != "no goal" || len(a.Evidence) != 1 {
		t.Fatalf("%+v %v", a, err)
	}
	if !r.req.Role.ReadOnly || len(r.req.Role.Skills) != 0 || !strings.Contains(r.req.Prompt, "--- document: d.md ---") {
		t.Fatalf("request %+v", r.req)
	}
	if !json.Valid(r.req.OutputSchema) {
		t.Fatal("the output schema is not JSON")
	}
}

func TestTheHarnessJudgeRefusesBadTurns(t *testing.T) {
	for name, r := range map[string]*reply{
		"extra field": {out: `{"answer":"pass","rationale":"","evidence":[],"x":1}`},
		"not json":    {out: `nope`},
		"budget stop": {out: `{"answer":"pass","rationale":"","evidence":[]}`, exit: harness.ExitBudget},
		"rate limit":  {out: `{}`, exit: harness.ExitRateLimit},
	} {
		if _, err := ask(t, r); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := (HarnessJudge{}).Judge(t.Context(), Request{}); err == nil {
		t.Error("no harness: want an error")
	}
}
