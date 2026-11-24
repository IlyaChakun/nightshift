package check_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/check"
)

const oneResult = `printf '%s' '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"t"}},"results":[{"ruleId":"R1","level":"error","message":{"text":"bad"}}]}]}' > "$NIGHTSHIFT_SARIF"; exit 1`

func run(t *testing.T, o check.Options) check.Report {
	t.Helper()
	o.Dir, o.Out = t.TempDir(), filepath.Join(t.TempDir(), "out")
	r, err := check.Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRunReportsNewFindings(t *testing.T) {
	r := run(t, check.Options{Tools: []check.Tool{{Name: "lint", Command: oneResult, Category: "code"}}})
	if len(r.New) != 1 || r.New[0].RuleID != "t.r1" || !check.Blocking(r.New) {
		t.Fatalf("report: %+v", r)
	}
}

func TestRunSkipsBaselineFindings(t *testing.T) {
	first := run(t, check.Options{Tools: []check.Tool{{Name: "lint", Command: oneResult, Category: "code"}}})
	r := run(t, check.Options{Baseline: first.All, Tools: []check.Tool{{Name: "lint", Command: oneResult, Category: "code"}}})
	if len(r.All) != 1 || len(r.New) != 0 {
		t.Fatalf("an old violation must not block: %+v", r)
	}
}

func TestRunTurnsFailuresIntoFindings(t *testing.T) {
	r := run(t, check.Options{
		Verify: "echo tests broke; exit 3",
		Tools: []check.Tool{
			{Name: "quiet", Command: "true", Category: "code"},
			{Name: "junk", Command: "echo nope > $NIGHTSHIFT_SARIF", Category: "security"},
		},
	})
	if len(r.New) != 2 || r.New[0].RuleID != "verify.failed" || r.New[1].RuleID != "junk.failed" {
		t.Fatalf("report: %+v", r.New)
	}
}

func TestRunRejectsUnsafeToolName(t *testing.T) {
	_, err := check.Run(t.Context(), check.Options{Dir: t.TempDir(), Out: t.TempDir(), Tools: []check.Tool{{Name: "../x", Command: "true"}}})
	if err == nil {
		t.Fatal("tool name with a path was accepted")
	}
}

func TestRunStopsOnTimeout(t *testing.T) {
	r := run(t, check.Options{Verify: "sleep 5", Timeout: 100 * time.Millisecond})
	if len(r.New) != 1 || r.New[0].RuleID != "verify.failed" {
		t.Fatalf("report: %+v", r)
	}
}
