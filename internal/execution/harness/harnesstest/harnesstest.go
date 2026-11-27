// Package harnesstest is the contract suite every model adapter under internal/execution/harness
// passes (docs/plan/architecture/ item A7): a result that is JSON, usage, the error mapping
// (limit, failure, unreadable output) and how a run ends when its context is canceled or runs out.
// It belongs to the testing plane: only tests import it.
package harnesstest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
)

// Reply is what the adapter's stub provider does in one turn.
type Reply int

// The replies a stub provider must be able to give.
const (
	// Success answers {"summary":"done"} and reports 10 input and 5 output tokens.
	Success Reply = iota
	// Limit stops the turn with the provider's plan or rate limit message.
	Limit
	// Failure ends the turn with a provider error that says "boom".
	Failure
	// Garbage prints output the adapter cannot read and exits with status 1.
	Garbage
	// Hang reads the prompt and never answers.
	Hang
)

// Adapter returns a harness whose provider gives reply. It runs once per case, in sequence:
// stub binaries written by parallel tests can fail with "text file busy".
type Adapter func(t *testing.T, reply Reply) harness.Harness

// stopWithin is how long a canceled run may take to return; the adapters give a child a grace
// period before they kill it, so this is generous on purpose.
const stopWithin = 30 * time.Second

// Run checks the contract on one adapter.
func Run(t *testing.T, adapter Adapter) {
	t.Helper()
	t.Run("result and usage", func(t *testing.T) { success(t, adapter(t, Success)) })
	t.Run("limit", func(t *testing.T) { limit(t, adapter(t, Limit)) })
	t.Run("failure keeps the reason", func(t *testing.T) { failure(t, adapter(t, Failure)) })
	t.Run("unreadable output is never done", func(t *testing.T) { garbage(t, adapter(t, Garbage)) })
	t.Run("cancel", func(t *testing.T) { canceled(t, adapter(t, Hang)) })
	t.Run("deadline is the budget", func(t *testing.T) { deadline(t, adapter(t, Hang)) })
}

// Request is the turn every case runs; an adapter's stub may read its schema.
func Request(t *testing.T) harness.RunRequest {
	t.Helper()
	return harness.RunRequest{
		Role: harness.RoleConfig{Name: "builder"}, Workspace: t.TempDir(), Prompt: "fix the bug",
		OutputSchema: []byte(`{"type":"object","properties":{"summary":{"type":"string"}}}`),
	}
}

func success(t *testing.T, h harness.Harness) {
	res, err := h.Run(t.Context(), Request(t))
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Summary string }
	if res.ExitReason != harness.ExitDone || json.Unmarshal(res.Output, &out) != nil || out.Summary != "done" {
		t.Errorf("want done with {\"summary\":\"done\"}, got %s %s", res.ExitReason, res.Output)
	}
	if res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 5 || res.Usage.LimitHit {
		t.Errorf("usage: %+v", res.Usage)
	}
}

func limit(t *testing.T, h harness.Harness) {
	res, err := h.Run(t.Context(), Request(t))
	if err != nil {
		t.Fatalf("a limit is a result, not an error: %v", err)
	}
	if res.ExitReason != harness.ExitRateLimit || !res.Usage.LimitHit {
		t.Errorf("want rate_limit with LimitHit, got %s %+v", res.ExitReason, res.Usage)
	}
}

func failure(t *testing.T, h harness.Harness) {
	res, err := h.Run(t.Context(), Request(t))
	if err != nil {
		t.Fatalf("a provider failure is a result, not an error: %v", err)
	}
	var out struct{ Error string }
	if res.ExitReason != harness.ExitError || json.Unmarshal(res.Output, &out) != nil || out.Error == "" {
		t.Errorf("want error with the reason in the output, got %s %s", res.ExitReason, res.Output)
	}
	if res.Usage.LimitHit {
		t.Error("a failure is not a limit")
	}
}

func garbage(t *testing.T, h harness.Harness) {
	res, err := h.Run(t.Context(), Request(t))
	if err == nil && res.ExitReason != harness.ExitError {
		t.Errorf("unreadable output passed as %s %s", res.ExitReason, res.Output)
	}
}

func canceled(t *testing.T, h harness.Harness) {
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(300*time.Millisecond, cancel)
	start := time.Now()
	_, err := h.Run(ctx, Request(t))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("a canceled run returns the cancellation, got %v", err)
	}
	if took := time.Since(start); took > stopWithin {
		t.Errorf("a canceled run took %s to return", took)
	}
}

func deadline(t *testing.T, h harness.Harness) {
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	res, err := h.Run(ctx, Request(t))
	if err != nil || res.ExitReason != harness.ExitBudget {
		t.Errorf("a run out of time ends as budget without an error, got %s, %v", res.ExitReason, err)
	}
	if took := time.Since(start); took > stopWithin {
		t.Errorf("a run out of time took %s to return", took)
	}
}

// HangStub writes an executable named name that reads the prompt and then sleeps, for the Hang
// reply of any CLI adapter.
func HangStub(t *testing.T, name string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\ncat >/dev/null\nexec sleep 600\n"), 0o700); err != nil { //nolint:gosec // G306: a test stub must be executable
		t.Fatal(err)
	}
	return bin
}
