package summary

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
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
	i2, i3 := strings.Index(out, "(#2)"), strings.Index(out, "(#3)")
	if i2 < 0 || i2 >= i3 || strings.Contains(out, "(#1)") {
		t.Fatalf("order must be gated, then unreviewed; docs and reviewed code are counted, not listed:\n%s", out)
	}
	for _, want := range []string{
		"Sessions and people merged 4 more: 1 on protected paths, 1 code changes without a recorded review",
		"Protected paths, look at these first (1):", "Code without a recorded independent review (1):",
		"https://github.com/example/app/pull/3", "approved: add a thing (by owner)", "*Work and results*", "*Conclusion*", "1 protected-path and 1 unreviewed code merges went in without you",
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
	for _, want := range []string{"*Needs you* (20)", "• 15 more", "• 292 more: https://github.com/example/app/pulls?q=is%3Apr+is%3Amerged+merged%3A%3E%3D"} {
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

func TestDigestHeadlinesTheFactoriesOwnWorkAndListsItFirst(t *testing.T) {
	t.Parallel()
	d := Digest{
		Summary: Summary{Since: time.Now()},
		Merges: []integration.MergedPR{
			{Number: 5, Title: "feat: by a task", URL: "https://github.com/example/app/pull/5", Reviewed: true},
			{Number: 6, Title: "feat: by a session", URL: "https://github.com/example/app/pull/6", Reviewed: true},
			{Number: 7, Title: "fix: by a session", URL: "https://github.com/example/app/pull/7", Labels: []string{"needs-owner-review"}},
		},
		Factory: map[string]bool{"example/app#5": true},
	}
	out := d.Format()
	for _, want := range []string{"The factory's own tasks merged 1 pull request. Sessions and people merged 2 more: 1 on protected paths, 0 code changes", "The factory's own work (1):"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if i5, i7 := strings.Index(out, "(#5)"), strings.Index(out, "(#7)"); i5 < 0 || i5 >= i7 {
		t.Errorf("order must be factory, then protected:\n%s", out)
	}
	none := Digest{Summary: Summary{Since: time.Now()}, Merges: d.Merges[1:]}.Format()
	if !strings.Contains(none, "The factory's own tasks merged 0 pull requests.") {
		t.Errorf("no factory work says so:\n%s", none)
	}
}

func TestMergesListNewestFirst(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	var merges []integration.MergedPR
	for i := 1; i <= 15; i++ { // oldest first, as the code host adapter returns them
		merges = append(merges, integration.MergedPR{Number: i, Title: "x", URL: fmt.Sprintf("https://github.com/example/app/pull/%d", i), Labels: []string{"needs-owner-review"}, MergedAt: at.Add(time.Duration(i) * time.Minute)})
	}
	out := Digest{Summary: Summary{Since: at}, Merges: merges}.Format()
	if !strings.Contains(out, "(#15)") || !strings.Contains(out, "(#8)") || strings.Contains(out, "(#7)") {
		t.Fatalf("the eight newest must show:\n%s", out)
	}
	if strings.Index(out, "(#15)") > strings.Index(out, "(#14)") {
		t.Fatalf("newest first:\n%s", out)
	}
}

func TestNeedsYouDropsATaskThatALaterOneReplaced(t *testing.T) {
	t.Parallel()
	task := func(id int64, title string, st flow.State) bookdb.BookTask {
		return bookdb.BookTask{ID: id, Title: title, State: string(st)}
	}
	got := needsYou([]bookdb.BookTask{
		task(1001, "docs: notes", flow.Released), // the redo
		task(1000, "docs: notes", flow.Escalated),
		task(1005, "feat: other", flow.Escalated),
		task(1006, "feat: again", flow.Escalated),
		task(1007, "feat: again", flow.HumanReview), // the later one waits too: only it shows
	})
	want := []Waiting{{"feat: other", flow.Escalated}, {"feat: again", flow.HumanReview}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
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
	for _, want := range []string{"good one (#5)", "partial (#6)", "example/b: 403 forbidden", "example/c: truncated", "incomplete"} {
		if !strings.Contains(out, want) {
			t.Errorf("digest lacks %q:\n%s", want, out)
		}
	}
}

func TestWorkAndResultsGroupsByAreaAndSaysWhatWasDone(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	pr := func(n int, title string, age time.Duration) integration.MergedPR {
		return integration.MergedPR{Number: n, Title: title, URL: fmt.Sprintf("https://github.com/example/app/pull/%d", n), Reviewed: true, MergedAt: at.Add(-age)}
	}
	d := Digest{Summary: Summary{Since: at.Add(-24 * time.Hour), Until: at}, Merges: []integration.MergedPR{
		pr(1, "feat(dispatch): wait for a near limit reset", 5*time.Hour),
		pr(2, "fix(dispatch): send a broken Lead reply back once", time.Hour),
		pr(3, "docs: status handoff", 2*time.Hour),
		pr(4, "Добавь раздел в runbook", 3*time.Hour),
	}}
	out := d.workBlock()
	for _, want := range []string{"• dispatch (2): send a broken Lead reply back once; wait for a near limit reset", "• docs (1): status handoff", "• other (1): Добавь раздел в runbook"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "dispatch") > strings.Index(out, "docs") {
		t.Errorf("the biggest area comes first:\n%s", out)
	}
}

func TestConclusionSaysWhatToDo(t *testing.T) {
	t.Parallel()
	green := Digest{Summary: Summary{Since: time.Now()}, Health: &Health{}}
	if out := green.Format(); !strings.Contains(out, "• Main is green, no open alerts, nothing idle.") || strings.Contains(out, "*Factory health*") {
		t.Errorf("a healthy factory gets one line, no detail:\n%s", out)
	}
	if out := green.Format(); !strings.Contains(out, "• No protected-path or unreviewed code merged.") || !strings.Contains(out, "• Nothing waits for you.") {
		t.Errorf("conclusion:\n%s", out)
	}
	red := Digest{Summary: Summary{Since: time.Now()}, Health: &Health{MainFailed: []string{"test"}}}
	if out := red.Format(); !strings.Contains(out, "• Look at: main is red.") || !strings.Contains(out, "*Factory health*") {
		t.Errorf("a problem shows with its detail:\n%s", out)
	}
}

func TestTheConclusionSurvivesALongMessage(t *testing.T) {
	t.Parallel()
	var merges []integration.MergedPR
	for i := 1; i <= 40; i++ {
		merges = append(merges, integration.MergedPR{Number: i, Title: "feat(x): " + strings.Repeat("word ", 30), URL: fmt.Sprintf("https://github.com/example/app/pull/%d", i), Labels: []string{"needs-owner-review"}})
	}
	var decisions []Decision
	for range 30 {
		decisions = append(decisions, Decision{Title: strings.Repeat("t ", 50), Event: "approve", At: time.Now()})
	}
	out := Digest{Summary: Summary{Since: time.Now()}, Merges: merges, Decisions: decisions, Health: &Health{MainFailed: []string{"x"}}}.Format()
	if n := len([]rune(out)); n > maxDigestRunes || !strings.HasSuffix(out, "• Look at: main is red.") {
		t.Fatalf("%d characters; the conclusion must stay last:\n%s", n, out)
	}
}

// Regression tests for the review of PR #324.

func TestTheConclusionSurvivesTheWorstCaseLength(t *testing.T) {
	t.Parallel()
	var merges []integration.MergedPR
	for i := 1; i <= 30; i++ {
		repo := fmt.Sprintf("some-organisation/repository-%d", i%2)
		merges = append(merges, integration.MergedPR{
			Number: i, URL: fmt.Sprintf("https://github.com/%s/pull/%d", repo, i), Labels: []string{"needs-owner-review"},
			Title: fmt.Sprintf("feat(scope%d): implement a considerably longer description of the change that goes on and on number %d here", i%9, i),
		})
	}
	waiting := make([]Waiting, 7)
	for i := range waiting {
		waiting[i] = Waiting{Title: strings.Repeat("waiting task ", 8), State: flow.Escalated}
	}
	d := Digest{
		Summary: Summary{Since: time.Now(), Waiting: map[flow.State]int64{flow.Escalated: 7}, NeedsYou: waiting},
		Merges:  merges, Health: &Health{MainFailed: []string{"r: ci"}},
	}
	out := d.Format()
	if n := len([]rune(out)); n > maxDigestRunes || !strings.Contains(out, "*Conclusion*") || !strings.HasSuffix(out, "• Look at: main is red.") {
		t.Fatalf("%d characters; the conclusion must survive:\n%s", n, out)
	}
}

func TestTheConclusionDoesNotClaimWhatItDidNotSee(t *testing.T) {
	t.Parallel()
	out := Digest{Summary: Summary{Since: time.Now()}, MergeErr: errors.New("403")}.conclusion()
	if strings.Contains(out, "No protected-path") || !strings.Contains(out, "merge list is incomplete") {
		t.Fatalf("an unreadable merge list is not a clean one:\n%s", out)
	}
}

func TestARedoneTaskInAnyActiveStateReplacesTheWaitingOne(t *testing.T) {
	t.Parallel()
	for _, st := range append(slices.Clone(activeStates), flow.Released, flow.MergedByTeam, flow.HandedOff, flow.Blocked) {
		if !slices.Contains(considered, st) {
			t.Errorf("a task in %s must replace a waiting one", st)
		}
	}
	got := needsYou([]bookdb.BookTask{
		{ID: 1, Title: "X", State: string(flow.Escalated)},
		{ID: 2, Title: "X", State: string(flow.Grilling)},
	})
	if len(got) != 0 {
		t.Fatalf("the redo is in work, nothing waits: %v", got)
	}
}

func TestOnlyKnownCommitTypesAreStrippedFromATitle(t *testing.T) {
	t.Parallel()
	for _, title := range []string{"Revert: add X", "WIP: x", "Merge: x"} {
		m := integration.MergedPR{Title: title}
		if what(m) != title || area(m) != "other" {
			t.Errorf("%q: what %q, area %q", title, what(m), area(m))
		}
	}
	m := integration.MergedPR{Title: "feat(a very long scope name that goes on and on): x"}
	if got := area(m); len([]rune(got)) > maxArea+1 {
		t.Errorf("an area name is bounded: %q", got)
	}
}

func TestOneLeftoverAreaIsSingular(t *testing.T) {
	t.Parallel()
	var merges []integration.MergedPR
	for i := 1; i <= maxAreas+1; i++ {
		merges = append(merges, integration.MergedPR{Number: i, Title: fmt.Sprintf("feat(a%d): x", i), URL: fmt.Sprintf("https://github.com/o/r/pull/%d", i), Reviewed: true})
	}
	if out := (Digest{Merges: merges}).workBlock(); !strings.Contains(out, "• 1 more in 1 other area\n") && !strings.HasSuffix(out, "• 1 more in 1 other area") {
		t.Fatalf("singular:\n%s", out)
	}
}
