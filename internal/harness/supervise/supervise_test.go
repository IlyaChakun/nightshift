package supervise_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/harness/supervise"
)

// script is a harness that plays events, then waits for its context, as a hung agent does.
type script struct {
	events []harness.Event
	hang   bool
	result harness.RunResult
}

func (s script) Run(ctx context.Context, req harness.RunRequest) (harness.RunResult, error) {
	for _, ev := range s.events {
		req.Emit(ev)
	}
	if s.hang {
		<-ctx.Done()
		return harness.RunResult{}, ctx.Err()
	}
	return s.result, nil
}

func call(tool, in string) harness.Event {
	return harness.Event{Kind: harness.EventToolCall, Tool: tool, Input: in}
}

func usage(in, out int, usd float64) harness.Event {
	return harness.Event{Kind: harness.EventUsage, Usage: harness.Usage{InputTokens: in, OutputTokens: out, CostUSD: usd}}
}

func stopped(t *testing.T, res harness.RunResult, err error, want string) {
	t.Helper()
	if err != nil || res.ExitReason != harness.ExitBudget {
		t.Fatalf("res %+v, err %v: want a stopped run", res, err)
	}
	var out map[string]string
	if json.Unmarshal(res.Output, &out) != nil || !strings.Contains(out["stopped"], want) {
		t.Fatalf("output %s lacks %q", res.Output, want)
	}
}

func TestRepeatedToolCallStopsTheRun(t *testing.T) {
	t.Parallel()
	s := supervise.Supervisor{Policy: supervise.Policy{RepeatLimit: 3}, Next: script{hang: true, events: []harness.Event{
		call("Bash", "go test"), call("Bash", "go test"), call("Edit", "a"), call("Bash", "go test"), call("Bash", "go test"), call("Bash", "go test"),
	}}}
	res, err := s.Run(t.Context(), harness.RunRequest{})
	stopped(t, res, err, "loop: Bash called 3 times")
}

func TestVariedCallsAreNotALoop(t *testing.T) {
	t.Parallel()
	want := harness.RunResult{ExitReason: harness.ExitDone}
	s := supervise.Supervisor{Policy: supervise.Policy{RepeatLimit: 2}, Next: script{result: want, events: []harness.Event{
		call("Bash", "a"), call("Bash", "b"), call("Bash", "a"),
	}}}
	res, err := s.Run(t.Context(), harness.RunRequest{})
	if err != nil || res.ExitReason != harness.ExitDone {
		t.Fatalf("res %+v, err %v", res, err)
	}
}

func TestTokenAndCostLimits(t *testing.T) {
	t.Parallel()
	s := supervise.Supervisor{Policy: supervise.Policy{Tokens: 100}, Next: script{hang: true, events: []harness.Event{usage(40, 20, 0), usage(30, 20, 0)}}}
	res, err := s.Run(t.Context(), harness.RunRequest{})
	stopped(t, res, err, "tokens: 110 used, limit 100")
	if res.Usage.InputTokens != 70 {
		t.Errorf("usage of the stopped run = %+v, want what was seen", res.Usage)
	}
	s = supervise.Supervisor{Policy: supervise.Policy{CostUSD: 1}, Next: script{hang: true, events: []harness.Event{usage(1, 1, 0.6), usage(1, 1, 0.6)}}}
	res, err = s.Run(t.Context(), harness.RunRequest{})
	stopped(t, res, err, "cost: $1.20 spent")
}

func TestSilenceStopsTheRun(t *testing.T) {
	t.Parallel()
	s := supervise.Supervisor{Policy: supervise.Policy{StallAfter: 50 * time.Millisecond}, Next: script{hang: true}}
	res, err := s.Run(t.Context(), harness.RunRequest{})
	stopped(t, res, err, "stall: no output for 50ms")
}

func TestZeroPolicyChangesNothing(t *testing.T) {
	t.Parallel()
	want := harness.RunResult{ExitReason: harness.ExitDone, SessionID: "s"}
	s := supervise.Supervisor{Next: script{result: want, events: []harness.Event{call("Bash", "x"), call("Bash", "x"), call("Bash", "x"), usage(1e6, 1e6, 99)}}}
	if res, err := s.Run(t.Context(), harness.RunRequest{}); err != nil || res.SessionID != "s" || res.ExitReason != harness.ExitDone {
		t.Fatalf("res %+v, err %v", res, err)
	}
}

func TestOutsideCancelIsNotAStop(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s := supervise.Supervisor{Policy: supervise.Policy{RepeatLimit: 3}, Next: script{hang: true}}
	if _, err := s.Run(ctx, harness.RunRequest{}); err == nil {
		t.Fatal("a canceled run must still return its error")
	}
}

type everyCall struct{}

func (everyCall) Check(ev harness.Event) string {
	if ev.Kind == harness.EventToolCall && ev.Tool == "Forbidden" {
		return "forbidden tool"
	}
	return ""
}

func TestExtraRule(t *testing.T) {
	t.Parallel()
	s := supervise.Supervisor{Extra: []func() supervise.Rule{func() supervise.Rule { return everyCall{} }},
		Next: script{hang: true, events: []harness.Event{call("Forbidden", "")}}}
	res, err := s.Run(t.Context(), harness.RunRequest{})
	stopped(t, res, err, "forbidden tool")
}
