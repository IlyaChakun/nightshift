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
	if i3 < 0 || i3 >= i2 || strings.Contains(out, "(#1)") {
		t.Fatalf("order must be unreviewed code, then protected paths; docs and reviewed code are counted, not listed:\n%s", out)
	}
	for _, want := range []string{
		"4 pull requests merged: 0 by the factory's own tasks, 4 by sessions and people.",
		"Protected paths (1):", "Code without a recorded independent review (1):",
		"https://github.com/example/app/pull/3", "approved: add a thing (by owner)", "*What changed*", "*Conclusion*",
		`Merged without you: 1 code change with no recorded review and 1 on protected paths. Both lists are under "Merged".`,
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
	if strings.Contains(out, "Everything else") || strings.Contains(out, "The factory's own work") {
		t.Errorf("a group with no line shown has no heading:\n%s", out)
	}
	if strings.Count(out, "/pull/") != maxMergeLines {
		t.Errorf("want %d merge lines:\n%s", maxMergeLines, out)
	}
}

func TestDigestCountsTheFactoriesOwnWorkAndListsItAfterWhatNeedsALook(t *testing.T) {
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
	for _, want := range []string{"3 pull requests merged: 1 by the factory's own tasks, 2 by sessions and people.", "The factory's own work (1):"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if i5, i7 := strings.Index(out, "(#5)"), strings.Index(out, "(#7)"); i7 < 0 || i7 >= i5 {
		t.Errorf("order must be protected, then the factory's own:\n%s", out)
	}
	none := Digest{Summary: Summary{Since: time.Now()}, Merges: d.Merges[1:]}.Format()
	if !strings.Contains(none, "2 pull requests merged: 0 by the factory's own tasks, 2 by sessions and people.") {
		t.Errorf("no factory work says so:\n%s", none)
	}
	if one := (Digest{Summary: Summary{Since: time.Now()}, Merges: d.Merges[:1]}).Format(); !strings.Contains(one, "1 pull request merged:") {
		t.Errorf("singular:\n%s", one)
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

func TestWhatChangedNamesNewFixedAndRevertedAndCountsUpkeep(t *testing.T) {
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
		pr(5, `Revert "feat: export"`, 4*time.Hour),
		pr(6, "test: a flaky case", 6*time.Hour),
		pr(7, "feat: export", 7*time.Hour),
	}}
	out := d.changedBlock()
	want := strings.Join([]string{
		"*What changed*",
		"New (2):", "• dispatch: wait for a near limit reset", "• export",
		"Fixed (1):", "• dispatch: send a broken Lead reply back once",
		"Reverted (1):", `• Revert "feat: export"`,
		"Other (1):", "• Добавь раздел в runbook",
		"Docs, tests and upkeep: 2",
	}, "\n")
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	// A pull request count per area of the code says what was busy, not what changed.
	if strings.Contains(out, "dispatch (") || strings.Contains(out, "docs (") {
		t.Errorf("no counts per area:\n%s", out)
	}
	var many []integration.MergedPR
	for i := 1; i <= maxChangeItems+2; i++ {
		many = append(many, pr(i, fmt.Sprintf("feat: thing %d", i), time.Duration(i)*time.Minute))
	}
	if out := (Digest{Merges: many}).changedBlock(); !strings.Contains(out, "New (5):\n• thing 1\n• thing 2\n• thing 3\n• 2 more") {
		t.Errorf("the newest are named and the rest counted:\n%s", out)
	}
	if out := (Digest{}).changedBlock(); out != "" {
		t.Errorf("nothing merged, nothing changed:\n%s", out)
	}
}

func TestConclusionSaysWhatToDo(t *testing.T) {
	t.Parallel()
	green := Digest{Summary: Summary{Since: time.Now()}, Health: &Health{}}.Format()
	if !strings.Contains(green, "\nNothing waits for you. Main is green, no open alerts, nothing idle.\n") || strings.Contains(green, "*Factory health*") {
		t.Errorf("a healthy factory gets one line in the head, no detail:\n%s", green)
	}
	if !strings.HasSuffix(green, "*Conclusion*\n• Nothing to do today.") {
		t.Errorf("a quiet day has a one-line conclusion:\n%s", green)
	}
	red := Digest{Summary: Summary{Since: time.Now()}, Health: &Health{MainFailed: []string{"test"}}}
	if out := red.Format(); !strings.Contains(out, "• Look at: main is red.") || !strings.Contains(out, "*Factory health*") || strings.Contains(out, "Nothing to do today") {
		t.Errorf("a problem shows with its detail:\n%s", out)
	}
	waiting := Digest{Summary: Summary{Since: time.Now(), Waiting: map[flow.State]int64{flow.Escalated: 2}}}
	if out := waiting.conclusion(); !strings.Contains(out, `• Start with "Needs you": 2 tasks wait for you.`) {
		t.Errorf("what waits comes first:\n%s", out)
	}
	for _, c := range []struct {
		merges []integration.MergedPR
		want   string
	}{
		{[]integration.MergedPR{{Title: "fix: x"}}, `• Merged without you: 1 code change with no recorded review, listed under "Merged".`},
		{[]integration.MergedPR{{Title: "docs: x", Labels: []string{gated}}}, `• Merged without you: 1 change on protected paths, listed under "Merged".`},
	} {
		if out := (Digest{Merges: c.merges}).conclusion(); !strings.Contains(out, c.want) {
			t.Errorf("want %q:\n%s", c.want, out)
		}
	}
}

// The digest of 2026-11-26 said "nothing waits for you" in the head and in the conclusion, and the
// counts of protected and unreviewed merges three times. Each fact has one place.
func TestEachFactIsSaidOnce(t *testing.T) {
	t.Parallel()
	var merges []integration.MergedPR
	for i := 1; i <= 7; i++ {
		merges = append(merges, integration.MergedPR{Number: i, Title: "fix: thing", URL: fmt.Sprintf("https://github.com/example/app/pull/%d", i)})
	}
	for i := 8; i <= 20; i++ {
		merges = append(merges, integration.MergedPR{Number: i, Title: "feat: thing", URL: fmt.Sprintf("https://github.com/example/app/pull/%d", i), Reviewed: true, Labels: []string{gated}})
	}
	out := Digest{Summary: Summary{Since: time.Now()}, Merges: merges, Health: &Health{}}.Format()
	for _, once := range []string{"Nothing waits for you", "Main is green", "7 code changes with no recorded review", "13 on protected paths"} {
		if n := strings.Count(out, once); n != 1 {
			t.Errorf("%q is said %d times:\n%s", once, n, out)
		}
	}
}

// The digest of 2026-11-26 told the owner to skim 33 unreviewed code merges "above", and none was
// listed: the factory's own work and the protected paths had used every line.
func TestEveryGroupTheConclusionCountsHasLinesInMerged(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	var merges []integration.MergedPR
	factory := map[string]bool{}
	add := func(n int, title string, reviewed bool, labels ...string) {
		merges = append(merges, integration.MergedPR{Number: n, Title: title, URL: fmt.Sprintf("https://github.com/example/app/pull/%d", n), Reviewed: reviewed, Labels: labels, MergedAt: at.Add(time.Duration(n) * time.Second)})
	}
	for n := 1; n <= 4; n++ {
		add(n, "feat: by a task", true)
		factory[fmt.Sprintf("example/app#%d", n)] = true
	}
	for n := 101; n <= 153; n++ {
		add(n, "feat: protected", true, gated)
	}
	for n := 201; n <= 233; n++ {
		add(n, "fix: unreviewed", false)
	}
	out := Digest{Summary: Summary{Since: at}, Merges: merges, Factory: factory}.mergedBlock()
	unreviewed, protected, own := strings.Index(out, "Code without a recorded independent review (33):"), strings.Index(out, "Protected paths (53):"), strings.Index(out, "The factory's own work (4):")
	if unreviewed < 0 || protected < unreviewed || own < protected {
		t.Fatalf("all three groups show, unreviewed code first:\n%s", out)
	}
	for _, want := range []string{"(#233)", "(#230)", "(#153)", "(#152)", "(#4)", "(#3)",
		"• all on protected paths: https://github.com/example/app/pulls?q=is%3Apr+is%3Amerged+label%3Aneeds-owner-review+merged%3A%3E%3D",
		"• 82 more: https://github.com/example/app/pulls?q="} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "/pull/"); n != maxMergeLines {
		t.Errorf("%d merge lines, want %d:\n%s", n, maxMergeLines, out)
	}
	for _, c := range []struct{ unreviewed, gated, factory, wantU, wantG, wantF int }{
		{33, 53, 4, 4, 2, 2}, {0, 15, 0, 0, 8, 0}, {3, 0, 0, 3, 0, 0}, {20, 1, 0, 7, 1, 0}, {1, 20, 9, 1, 5, 2}, {0, 0, 9, 0, 0, 2},
	} {
		r := mergeRooms(c.unreviewed, c.gated, c.factory)
		if r[groupUnreviewed] != c.wantU || r[groupGated] != c.wantG || r[groupFactory] != c.wantF {
			t.Errorf("rooms for %+v: %v", c, r)
		}
	}
}

// The owner reads the times, so they are in the zone the caller gives, not in the server's.
func TestDigestTimesAreInTheZoneOfItsWindow(t *testing.T) {
	t.Parallel()
	zone := time.FixedZone("CET", 3600)
	until := time.Date(2026, 11, 26, 9, 0, 0, 0, zone)
	d := Digest{
		Summary:   Summary{Since: until.Add(-24 * time.Hour), Until: until},
		Decisions: []Decision{{Title: "add a thing", Event: "approve", At: time.Date(2026, 11, 26, 2, 30, 0, 0, time.UTC)}},
	}
	out := d.Format()
	for _, want := range []string{"last 24 h (since 25 Nov 09:00 CET)", "• 26 Nov 03:30 approved: add a thing"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
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
		repo := fmt.Sprintf("some-organization/repository-%d", i%2)
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
		if what(m) != title {
			t.Errorf("%q: what %q", title, what(m))
		}
	}
	for title, want := range map[string]change{"Revert: add X": changeReverted, "WIP: x": changeOther, "perf: x": changeUpkeep, "feat!: x": changeNew} {
		if got := changeOf(integration.MergedPR{Title: title}); got != want {
			t.Errorf("%q: change %d, want %d", title, got, want)
		}
	}
	m := integration.MergedPR{Title: "feat(a very long scope name that goes on and on): " + strings.Repeat("word ", 30)}
	if got := whatScoped(m, maxChangeWhat); len([]rune(got)) > maxChangeWhat+1 {
		t.Errorf("a line item is bounded: %q", got)
	}
}

func TestPlanBlockNamesTheOpenFilesInPlanOrder(t *testing.T) {
	t.Parallel()
	plan := []integration.PlanProgress{
		{Name: "Stream — other doors", Done: 2, Total: 5},
		{Name: "Phase 10 — New projects", Done: 0, Total: 3},
		{Name: "Phase 5 — Lead: planning and support", Done: 14, Total: 20},
		{Name: "Phase 3 — Builder, checks, gates", Done: 9, Total: 9},
		{Name: "Phase 6 — Verifier, preview, production", Done: 3, Total: 9},
		{Name: "Empty", Done: 0, Total: 0},
	}
	want := strings.Join([]string{
		"*Plan*",
		"• Phase 5 — Lead: planning and support: 14 of 20 done",
		"• Phase 6 — Verifier, preview, production: 3 of 9 done",
		"• Phase 10 — New projects: 0 of 3 done",
		"• Stream — other doors: 2 of 5 done",
	}, "\n")
	if out := planBlock(plan); out != want {
		t.Fatalf("phases by number, then the rest; no finished or empty file:\n%s\nwant:\n%s", out, want)
	}
	more := append(slices.Clone(plan), integration.PlanProgress{Name: "Stream — tenants", Done: 1, Total: 2})
	if out := planBlock(more); !strings.HasSuffix(out, "• 1 more plan file with open items") || strings.Contains(out, "tenants") {
		t.Fatalf("over the cap the rest is counted:\n%s", out)
	}
	if out := (Digest{Plan: plan}).Format(); !strings.Contains(out, "\n---\n*Plan*\n• Phase 5") {
		t.Fatalf("a quiet night still shows the plan, as a block of its own:\n%s", out)
	}
	if out := planBlock([]integration.PlanProgress{{Name: "Phase 3", Done: 9, Total: 9}}); out != "" {
		t.Fatalf("a finished plan says nothing:\n%s", out)
	}
}
