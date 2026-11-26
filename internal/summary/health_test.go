package summary

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/integration"
)

type healthSrc struct {
	alerts    []integration.Alert
	alertsErr error
	prs       []integration.PRActivity
	failed    []string
}

func (h healthSrc) OpenAlerts(context.Context, string) ([]integration.Alert, error) {
	return h.alerts, h.alertsErr
}

func (h healthSrc) OpenPullRequests(context.Context, string) ([]integration.PRActivity, error) {
	return h.prs, nil
}

func (h healthSrc) DefaultBranchFailures(context.Context, string) ([]string, error) {
	return h.failed, nil
}

var now = time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)

func TestHealthNamesOldAlertsIdlePRsAndARedMain(t *testing.T) {
	t.Parallel()
	src := healthSrc{
		alerts: []integration.Alert{
			{Number: 2, Rule: "r/b", Severity: "medium", Path: "b.go", Created: now.Add(-3 * time.Hour)},
			{Number: 1, Rule: "r/a", Severity: "high", Path: "a.go", Created: now.Add(-40 * time.Hour)},
		},
		prs: []integration.PRActivity{
			{Number: 7, Title: "stale", Updated: now.Add(-30 * time.Hour), Draft: true},
			{Number: 8, Title: "fresh", Updated: now.Add(-time.Hour)},
		},
		failed: []string{"verify"},
	}
	out := BuildHealth(t.Context(), src, []string{"o/r"}, now).Format()
	for _, want := range []string{"main is RED: o/r: verify", "2 code-scanning alert(s) open", "#1 high r/a in a.go", "1 pull request(s) idle", "#7 stale (draft)"} {
		if !strings.Contains(out, want) {
			t.Errorf("health lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "#8") {
		t.Errorf("a fresh pull request must not be listed:\n%s", out)
	}
	if strings.Index(out, "#1 high") > strings.Index(out, "#2 medium") {
		t.Errorf("worst alert first:\n%s", out)
	}
}

func TestHealthSaysUnknownWhenAPartCannotBeRead(t *testing.T) {
	t.Parallel()
	out := BuildHealth(t.Context(), healthSrc{alertsErr: errors.New("403")}, []string{"o/r"}, now).Format()
	if !strings.Contains(out, "code-scanning alerts: unknown") || strings.Contains(out, "none open") {
		t.Fatalf("an unreadable list must not read as an empty one:\n%s", out)
	}
	if !strings.Contains(out, "main: green") || !strings.Contains(out, "none idle") {
		t.Fatalf("the other signals still report:\n%s", out)
	}
	if BuildHealth(t.Context(), nil, []string{"o/r"}, now) != nil {
		t.Fatal("no source means no health part")
	}
}

func TestDigestAppendsHealth(t *testing.T) {
	t.Parallel()
	h := BuildHealth(t.Context(), healthSrc{}, []string{"o/r"}, now)
	// A healthy factory shows the one-line verdict; a problem adds the detail (TestConclusionSaysWhatToDo).
	if out := (Digest{Health: h}).Format(); !strings.Contains(out, "Main is green") {
		t.Fatalf("digest lacks the health verdict:\n%s", out)
	}
	h.MainFailed = []string{"test"}
	if out := (Digest{Health: h}).Format(); !strings.Contains(out, "Factory health") {
		t.Fatalf("digest lacks health detail:\n%s", out)
	}
}
