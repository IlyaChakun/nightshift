package summary

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func TestDigestListsGatedAndUnreviewedMergesFirstAndNamesTheRule(t *testing.T) {
	t.Parallel()
	d := Digest{
		Summary: Summary{Since: time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)},
		Merges: []integration.MergedPR{
			{Number: 1, Title: "docs: notes", URL: "https://github.com/example/app/pull/1", Reviewed: true, Reason: "Explains the package."},
			{Number: 2, Title: "feat: schema field", URL: "https://github.com/example/app/pull/2", Reviewed: true, Labels: []string{"needs-owner-review"}, MergedBy: "bot"},
			{Number: 3, Title: "fix: thing", URL: "https://github.com/example/app/pull/3"},
			{Number: 4, Title: "docs: unreviewed docs need none", URL: "https://github.com/example/app/pull/4"},
		},
		Decisions: []Decision{{TaskID: 7, Title: "add a thing", Actor: "owner", Event: "approve", From: "spec", To: "queued", At: time.Date(2026, 11, 25, 2, 0, 0, 0, time.UTC)}},
	}
	out := d.Format()
	i2, i3, i1 := strings.Index(out, "#2 "), strings.Index(out, "#3 "), strings.Index(out, "#1 ")
	if i2 < 0 || i2 >= i3 || i3 >= i1 {
		t.Fatalf("order must be gated, unreviewed, rest:\n%s", out)
	}
	for _, want := range []string{
		"1 on protected paths, 1 code changes without a recorded review",
		"Protected paths, look at these first (1):", "Code without a recorded independent review (1):",
		"https://github.com/example/app/pull/3", "approved: add a thing (by owner)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("digest lacks %q:\n%s", want, out)
		}
	}
	// The flag text is said once per group, never on every line; ids and states stay out.
	for _, bad := range []string{"[gated", "no independent review recorded", "#7", "spec -> queued", "why:"} {
		if strings.Contains(out, bad) {
			t.Errorf("digest still has %q:\n%s", bad, out)
		}
	}
	if strings.Count(out, "\n---\n") < 2 {
		t.Errorf("blocks need rules between them:\n%s", out)
	}
	if !strings.HasPrefix(out, "*Morning digest* · since 25 Nov 00:00 UTC\n") {
		t.Errorf("the summary comes first:\n%s", out)
	}
}

func TestDigestStaysShortHoweverManyMerges(t *testing.T) {
	t.Parallel()
	var merges []integration.MergedPR
	var decisions []Decision
	for i := 1; i <= 300; i++ {
		m := integration.MergedPR{Number: i, Title: strings.Repeat("a long title word ", 20), URL: fmt.Sprintf("https://github.com/example/app/pull/%d", i)}
		if i%3 == 0 {
			m.Labels = []string{"needs-owner-review"}
		}
		merges = append(merges, m)
		decisions = append(decisions, Decision{Title: strings.Repeat("title ", 40), Event: "approve", At: time.Now()})
	}
	waiting := make([]Waiting, 20)
	for i := range waiting {
		waiting[i] = Waiting{Title: strings.Repeat("waiting ", 30), State: flow.Escalated}
	}
	d := Digest{
		Summary: Summary{Since: time.Now().Add(-24 * time.Hour), Until: time.Now(), Waiting: map[flow.State]int64{flow.Escalated: 20}, NeedsYou: waiting},
		Merges:  merges, Decisions: decisions,
	}
	out := d.Format()
	if n := len([]rune(out)); n > maxDigestRunes {
		t.Fatalf("%d characters, cap %d", n, maxDigestRunes)
	}
	if n := strings.Count(out, "\n"); n > 45 {
		t.Fatalf("%d lines:\n%s", n, out)
	}
	for _, want := range []string{"*Needs you* (20)", "• 15 more", "• 290 more: https://github.com/example/app/pulls?q=is%3Apr+is%3Amerged+merged%3A%3E%3D", "• 295 more"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Everything else") || strings.Contains(out, "Code without a recorded") {
		t.Errorf("a group with no line shown has no heading:\n%s", out)
	}
	if strings.Count(out, "/pull/") != maxMergeLines {
		t.Errorf("want %d merge lines:\n%s", maxMergeLines, out)
	}
}

func TestDigestSaysWhatTheFactoryMadeItself(t *testing.T) {
	t.Parallel()
	d := Digest{
		Summary: Summary{Since: time.Now()},
		Merges: []integration.MergedPR{
			{Number: 5, Title: "feat: by a task", URL: "https://github.com/example/app/pull/5", Reviewed: true},
			{Number: 6, Title: "feat: by a session", URL: "https://github.com/example/app/pull/6", Reviewed: true},
		},
		Factory: map[string]bool{"example/app#5": true},
	}
	if out := d.Format(); !strings.Contains(out, "2 pull requests merged (1 by the factory's own tasks).") {
		t.Fatalf("origin count:\n%s", out)
	}
}

func TestDigestSaysWhenTheMergeListCannotBeRead(t *testing.T) {
	t.Parallel()
	out := Digest{MergeErr: errors.New("403")}.Format()
	if !strings.Contains(out, "incomplete") || !strings.Contains(out, "Merged pull requests: unknown") || strings.Contains(out, "No pull requests") {
		t.Fatalf("an unreadable list must not read as an empty one:\n%s", out)
	}
	if out := (Digest{}).Format(); !strings.Contains(out, "No pull requests merged.") {
		t.Fatalf("an empty window says so:\n%s", out)
	}
}

// The window is named in hours and dated, so a window that is not the last day shows at once
// (the digest of 2026-11-25 read "Since 24 Nov 19:58 UTC" and looked like a wrong window).
func TestDigestNamesItsWindow(t *testing.T) {
	t.Parallel()
	until := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		since time.Time
		want  string
	}{
		{until.Add(-24 * time.Hour), "last 24 h (since 25 Nov 08:00 UTC)"},
		{until.Add(-90 * time.Minute), "last 1.5 h (since 26 Nov 06:30 UTC)"},
		{until.Add(-30 * time.Minute), "last 30 min (since 26 Nov 07:30 UTC)"},
		{until.Add(-48 * time.Hour), "last 48 h (since 24 Nov 08:00 UTC)"},
	} {
		if out := (Digest{Summary: Summary{Since: c.since, Until: until}}).Format(); !strings.Contains(out, "*Morning digest* · "+c.want) {
			t.Errorf("want %q:\n%s", c.want, out)
		}
	}
}

type logs map[string]func() ([]integration.MergedPR, error)

func (l logs) MergedSince(_ context.Context, repo string, _ time.Time) ([]integration.MergedPR, error) {
	return l[repo]()
}

func TestAFailingRepositoryDoesNotHideTheMergesOfAnother(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	log := logs{
		"example/a": func() ([]integration.MergedPR, error) {
			return []integration.MergedPR{{Number: 5, Title: "good one", URL: "https://x/5", Labels: []string{"needs-owner-review"}}}, nil
		},
		"example/b": func() ([]integration.MergedPR, error) { return nil, errors.New("403 forbidden") },
		"example/c": func() ([]integration.MergedPR, error) {
			return []integration.MergedPR{{Number: 6, Title: "partial", URL: "https://x/6"}}, errors.New("truncated")
		},
	}
	d, err := BuildDigest(t.Context(), pool, log, "example-solo", []string{"example/a", "example/b", "example/c"}, time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	out := d.Format()
	for _, want := range []string{"#5 good one", "#6 partial", "example/b: 403 forbidden", "example/c: truncated", "incomplete"} {
		if !strings.Contains(out, want) {
			t.Errorf("digest lacks %q:\n%s", want, out)
		}
	}
}
