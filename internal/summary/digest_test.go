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

// lineOf is where the line of pull request n starts in out, -1 when it is not listed: a merge line
// ends with the page of its pull request.
func lineOf(out string, n int) int { return strings.Index(out+"\n", fmt.Sprintf("/pull/%d\n", n)) }

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
	i1, i2, i3 := lineOf(out, 1), lineOf(out, 2), lineOf(out, 3)
	if i2 < 0 || i2 >= i3 || i3 >= i1 || !strings.Contains(out, "Reviewed code and docs (2):") {
		t.Fatalf("order must be protected paths (ADR 0013), then unreviewed code, then on a quiet day the rest:\n%s", out)
	}
	if strings.Contains(out, "(#") {
		t.Errorf("the link carries the number; it is not written twice:\n%s", out)
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
	if i5, i7 := lineOf(out, 5), lineOf(out, 7); i7 < 0 || i7 >= i5 {
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
	if lineOf(out, 15) < 0 || lineOf(out, 8) < 0 || lineOf(out, 7) >= 0 {
		t.Fatalf("the eight newest must show:\n%s", out)
	}
	if lineOf(out, 15) > lineOf(out, 14) {
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
	if !strings.Contains(out, "\nThe merge list is incomplete: 403\n") || !strings.Contains(out, "Merged pull requests: unknown") || strings.Contains(out, "No pull requests") {
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
	for _, want := range []string{"• good one https://x/5", "• partial https://x/6", "example/b: 403 forbidden", "example/c: truncated", "incomplete"} {
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
		pr(8, "Revert button on the task card", 8*time.Hour),
		pr(9, "chore!: drop the old flag", 9*time.Hour),
	}}
	out := d.changedBlock()
	want := strings.Join([]string{
		"*What changed*",
		// "export" was taken back in the same window, so it is not new; a title that only starts
		// with "Revert" is not a revert; upkeep that breaks something is named, not counted.
		"New (1):", "• dispatch: wait for a near limit reset",
		"Fixed (1):", "• dispatch: send a broken Lead reply back once",
		"Reverted (1):", `• Revert "feat: export"`,
		"Other (3):", "• Добавь раздел в runbook", "• Revert button on the task card", "• drop the old flag",
		"Docs, tests and upkeep: 2",
	}, "\n")
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if got, want := d.changedCounts(), "*What changed*\nnew 1, fixed 1, reverted 1, other 3, docs, tests and upkeep 2."; got != want {
		t.Errorf("the one-line form: got %q, want %q", got, want)
	}
	for title, want := range map[string]change{
		"revert: the export": changeReverted, "Revert(api)!: the export": changeReverted, `Revert "fix: x"`: changeReverted,
		"revertible migrations": changeOther, " feat: leading space": changeNew, "fix(api)!: breaking fix": changeFixed, "docs: x": changeUpkeep,
	} {
		if got := changeOf(integration.MergedPR{Title: title}); got != want {
			t.Errorf("%q is kind %d, want %d", title, got, want)
		}
	}
	// A pull request count per area of the code says what was busy, not what changed.
	if strings.Contains(out, "dispatch (") || strings.Contains(out, "docs (") {
		t.Errorf("no counts per area:\n%s", out)
	}
	var many []integration.MergedPR
	for i := maxChangeItems + 2; i >= 1; i-- { // oldest first, as the code host returns them
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
	// What is wrong and what waits are in the head and in their own blocks; a conclusion that had
	// nothing else to say is left out.
	red := Digest{Summary: Summary{Since: time.Now()}, Health: &Health{MainFailed: []string{"test"}}}
	if out := red.Format(); !strings.Contains(out, "Nothing waits for you. Look at: main is red.") || !strings.Contains(out, "*Factory health*") || strings.Contains(out, "*Conclusion*") {
		t.Errorf("a problem shows in the head with its detail, and is not repeated:\n%s", out)
	}
	waiting := Digest{Summary: Summary{Since: time.Now(), Waiting: map[flow.State]int64{flow.Escalated: 2}}}
	if out := waiting.conclusion(true); out != "" {
		t.Errorf("the head says what waits; the conclusion does not repeat it:\n%s", out)
	}
	for _, c := range []struct {
		merges []integration.MergedPR
		want   string
	}{
		{[]integration.MergedPR{{Title: "fix: x"}}, "*Conclusion*\n• Merged without you: 1 code change with no recorded review."},
		{[]integration.MergedPR{{Title: "docs: x", Labels: []string{gated}}}, "*Conclusion*\n• Merged without you: 1 change on protected paths."},
	} {
		d := Digest{Merges: c.merges}
		if out := d.conclusion(true); out != c.want+` They are listed under "Merged".` {
			t.Errorf("want %q with the pointer:\n%s", c.want, out)
		}
		if out := d.conclusion(false); out != c.want {
			t.Errorf("want %q without the pointer when the lists may be cut:\n%s", c.want, out)
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
	// The same holds on a bad day: waiting tasks, a red main and a merge list that could not be read.
	bad := Digest{
		Summary: Summary{Since: time.Now(), Waiting: map[flow.State]int64{flow.Escalated: 3}, NeedsYou: []Waiting{{"a", flow.Escalated}, {"b", flow.Escalated}, {"c", flow.Escalated}}},
		Merges:  merges, Health: &Health{MainFailed: []string{"test"}}, MergeErr: errors.New("example/app: 503"),
	}.Format()
	for _, once := range []string{"3 need you", "Look at:", "incomplete", "*Needs you* (3)", "main is RED"} {
		if n := strings.Count(bad, once); n != 1 {
			t.Errorf("%q is said %d times:\n%s", once, n, bad)
		}
	}
	if strings.Contains(bad, "wait for you") || strings.Contains(bad, "*Conclusion*") {
		t.Errorf("the conclusion repeats the head:\n%s", bad)
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
	for n := 301; n <= 305; n++ {
		add(n, "docs: notes", true)
	}
	d := Digest{Summary: Summary{Since: at}, Merges: merges, Factory: factory}
	out := d.mergedBlock(maxMergeLines)
	unreviewed, protected, own := strings.Index(out, "Code without a recorded independent review (33):"), strings.Index(out, "Protected paths (53):"), strings.Index(out, "The factory's own work (4):")
	if protected < 0 || unreviewed < protected || own < unreviewed {
		t.Fatalf("all three groups show, protected paths first:\n%s", out)
	}
	for _, n := range []int{233, 230, 153, 152, 4, 3} {
		if lineOf(out, n) < 0 {
			t.Errorf("lacks pull request %d:\n%s", n, out)
		}
	}
	for _, want := range []string{
		"• all on protected paths: https://github.com/example/app/pulls?q=is%3Apr+is%3Amerged+label%3Aneeds-owner-review+merged%3A%3E%3D",
		"• 87 more: https://github.com/example/app/pulls?q="} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, groupTitle[groupOther]) {
		t.Errorf("on a busy day the rest is counted, not listed:\n%s", out)
	}
	if n := strings.Count(out, "/pull/"); n != maxMergeLines {
		t.Errorf("%d merge lines, want %d:\n%s", n, maxMergeLines, out)
	}
	// With the smallest budget each group the conclusion counts still has a line under its title.
	small := d.mergedBlock(minMergeLines)
	if n := strings.Count(small, "/pull/"); n != minMergeLines || lineOf(small, 233) < 0 || lineOf(small, 153) < 0 {
		t.Errorf("%d merge lines, want the newest of each counted group:\n%s", n, small)
	}
	for _, c := range []struct{ budget, unreviewed, gated, factory, other, wantU, wantG, wantF, wantO int }{
		{8, 33, 53, 4, 5, 4, 2, 2, 0}, {8, 0, 15, 0, 0, 0, 8, 0, 0}, {8, 3, 0, 0, 0, 3, 0, 0, 0}, {8, 20, 1, 0, 0, 7, 1, 0, 0},
		{8, 1, 20, 9, 0, 1, 5, 2, 0}, {8, 0, 0, 9, 3, 0, 0, 8, 0}, {8, 1, 1, 1, 9, 1, 1, 1, 5}, {2, 33, 53, 4, 5, 1, 1, 0, 0}, {3, 0, 53, 4, 0, 0, 1, 2, 0},
	} {
		var sizes [groupOther + 1]int
		sizes[groupUnreviewed], sizes[groupGated], sizes[groupFactory], sizes[groupOther] = c.unreviewed, c.gated, c.factory, c.other
		r := mergeRooms(c.budget, sizes)
		if r[groupUnreviewed] != c.wantU || r[groupGated] != c.wantG || r[groupFactory] != c.wantF || r[groupOther] != c.wantO {
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
	if n := len([]rune(out)); n > maxDigestRunes || !strings.HasSuffix(out, `• Merged without you: 40 changes on protected paths. They are listed under "Merged".`) || !strings.Contains(out, "Look at: main is red.") {
		t.Fatalf("%d characters; the conclusion must stay last and the head must say what is wrong:\n%s", n, out)
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
	if n := len([]rune(out)); n > maxDigestRunes || !strings.HasSuffix(out, `*Conclusion*`+"\n"+`• Merged without you: 30 changes on protected paths. They are listed under "Merged".`) {
		t.Fatalf("%d characters; the conclusion must survive:\n%s", n, out)
	}
}

func TestTheConclusionDoesNotClaimWhatItDidNotSee(t *testing.T) {
	t.Parallel()
	d := Digest{Summary: Summary{Since: time.Now()}, MergeErr: errors.New("403"), Merges: []integration.MergedPR{{Title: "fix: x"}}}
	if out := d.conclusion(true); out != "" {
		t.Fatalf("an unreadable merge list is not a clean one, and its count is not the whole count:\n%s", out)
	}
	if out := d.Format(); strings.Count(out, "incomplete") != 1 || strings.Contains(out, "Nothing to do today") {
		t.Fatalf("the head says once that the list is incomplete:\n%s", out)
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
	// Over the cap the files with the most open items stay, in plan order, and the rest is counted:
	// a nearly finished phase must not hide the one with the work.
	more := append(slices.Clone(plan),
		integration.PlanProgress{Name: "Phase 4b — night queue", Done: 0, Total: 4},
		integration.PlanProgress{Name: "Phase 7 — Self-improvement", Done: 0, Total: 12},
		integration.PlanProgress{Name: "Phase 8 — Several tenants", Done: 5, Total: 6},
		integration.PlanProgress{Name: "Stream — tenants", Done: 1, Total: 2})
	wantMore := strings.Join([]string{
		"*Plan*",
		"• Phase 4b — night queue: 0 of 4 done",
		"• Phase 5 — Lead: planning and support: 14 of 20 done",
		"• Phase 6 — Verifier, preview, production: 3 of 9 done",
		"• Phase 7 — Self-improvement: 0 of 12 done",
		"• Phase 10 — New projects: 0 of 3 done",
		"• Stream — other doors: 2 of 5 done",
		"• 2 more plan files with open items",
	}, "\n")
	if out := planBlock(more); out != wantMore {
		t.Fatalf("over the cap:\n%s\nwant:\n%s", out, wantMore)
	}
	if out := (Digest{Plan: plan}).Format(); !strings.Contains(out, "\n---\n*Plan*\n• Phase 5") {
		t.Fatalf("a quiet night still shows the plan, as a block of its own:\n%s", out)
	}
	if out := planBlock([]integration.PlanProgress{{Name: "Phase 3", Done: 9, Total: 9}}); out != "" {
		t.Fatalf("a finished plan says nothing:\n%s", out)
	}
}

// Regression tests for the review of PR #365.

// busyNight is the night the review measured: 90 merges with long titles in one repository (28
// unreviewed, 53 on protected paths, 4 by the factory), six open plan files.
func busyNight() Digest {
	at := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	d := Digest{Summary: Summary{Since: at.Add(-24 * time.Hour), Until: at}, Factory: map[string]bool{}, Health: &Health{}}
	for n := 1; n <= 90; n++ {
		m := integration.MergedPR{
			Number: n, URL: fmt.Sprintf("https://github.com/example/app/pull/%d", n), MergedAt: at.Add(-time.Duration(n) * time.Minute), Reviewed: true,
			Title: fmt.Sprintf("feat(scope%d): a change with a title of ordinary length, about seventy characters %d", n%7, n),
		}
		switch {
		case n <= 28:
			m.Reviewed, m.Title = false, "fix"+m.Title[4:]
		case n <= 81:
			m.Labels = []string{gated}
		case n <= 85:
			d.Factory[fmt.Sprintf("example/app#%d", n)] = true
		default:
			m.Title = "docs" + m.Title[4:]
		}
		d.Merges = append(d.Merges, m)
	}
	for i, name := range []string{"Phase 3 — Builder", "Phase 4 — Night queue", "Phase 5 — Lead", "Phase 6 — Verifier", "Stream — doors", "Stream — tenants"} {
		d.Plan = append(d.Plan, integration.PlanProgress{Name: name, Done: i, Total: 9})
	}
	return d
}

// A block gives way to the cap only when it does not fit: the first version dropped "Plan" and
// "Decisions" on every busy night, "Plan" even when it would have fit, and lost the health detail
// of a red main.
func TestABusyNightKeepsEveryBlockThatFits(t *testing.T) {
	t.Parallel()
	d := busyNight()
	d.Decisions = []Decision{{Title: "add a thing", Event: "approve", At: d.Summary.Until}, {Title: "drop a thing", Event: "reject", At: d.Summary.Until}}
	out := d.Format()
	for _, want := range []string{"*What changed*", "*Plan*", "*Merged* (90)", "*Decisions* (2)", "*Conclusion*"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q (%d characters):\n%s", want, len([]rune(out)), out)
		}
	}
	// More to say than fits: waiting tasks, a red main with its detail, many decisions.
	for i := range 5 {
		d.Summary.NeedsYou = append(d.Summary.NeedsYou, Waiting{Title: fmt.Sprintf("a task that waits for the owner with a title of some length %d", i), State: flow.Escalated})
	}
	d.Summary.Waiting = map[flow.State]int64{flow.Escalated: 5}
	d.Health = &Health{MainFailed: []string{"example/app: test, lint"}}
	for i := range 5 {
		d.Decisions = append(d.Decisions, Decision{Title: strings.Repeat("a long decision title ", 5) + fmt.Sprint(i), Event: "approve", At: d.Summary.Until})
	}
	out = d.Format()
	if n := len([]rune(out)); n > maxDigestRunes {
		t.Fatalf("%d characters, cap %d", n, maxDigestRunes)
	}
	// What stays: the health detail, what changed (its counts, the titles do not fit), the plan.
	for _, want := range []string{"*Needs you* (5)", "*Factory health*\n• main is RED: example/app: test, lint", "*What changed*\nnew 57, fixed 28, docs, tests and upkeep 5.", "*Plan*", "*Merged* (90)", "*Conclusion*"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
}

// The order in which the optional blocks are tried, and what "fits" means.
func TestBlocksAreAddedTheMostUsefulFirstWhileTheyFit(t *testing.T) {
	t.Parallel()
	d := busyNight()
	d.Health = &Health{MainFailed: []string{"example/app: test"}}
	d.Decisions = []Decision{{Title: "add a thing", Event: "approve", At: d.Summary.Until}}
	opt := d.optional()
	want := []optionalBlock{
		{blockHealth, "*Factory health*\n"}, {blockChanged, "*What changed*\nnew 57,"}, {blockPlan, "*Plan*\n"},
		{blockChanged, "*What changed*\nNew (57):\n"}, {blockDecisions, "*Decisions* (1)\n"},
	}
	if len(opt) != len(want) {
		t.Fatalf("%d optional blocks, want %d", len(opt), len(want))
	}
	for i, w := range want {
		if opt[i].at != w.at || !strings.HasPrefix(opt[i].text, w.text) {
			t.Errorf("optional block %d is at %d and starts %.30q; want place %d and %q", i, opt[i].at, opt[i].text, w.at, w.text)
		}
	}
	x := func(n int) string { return strings.Repeat("x", n) }
	room := 100 + 2*len(sep)
	// A block that does not fit is skipped and a smaller one after it still shows.
	blocks := []string{x(40), "", "", ""}
	fit(blocks, room, []optionalBlock{{1, x(50)}, {2, x(20)}, {3, x(10)}})
	if !slices.Equal(blocks, []string{x(40), x(50), "", x(10)}) {
		t.Errorf("40 + 50 fit in 100, 20 more do not, 10 more do: %q", blocks)
	}
	// A larger text for a place that is taken replaces it only when it fits.
	blocks = []string{x(40), "", "", ""}
	fit(blocks, room, []optionalBlock{{1, x(10)}, {2, x(30)}, {1, x(40)}, {1, x(30)}})
	if !slices.Equal(blocks, []string{x(40), x(30), x(30), ""}) {
		t.Errorf("the counts stay when the titles do not fit, and give way when they do: %q", blocks)
	}
}

// The conclusion points at the lists under "Merged", so both must be there with a line each, also
// when long repository names and waiting tasks leave little room (the review cut the protected
// list with 61-character names and lost it whole with 91).
func TestTheConclusionPointsOnlyAtListsThatAreThere(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	cut := 0
	for _, nameLen := range []int{20, 41, 61, 91, 150, 200, 400} {
		out := twoLongRepositories(at, nameLen).Format()
		if n := len([]rune(out)); n > maxDigestRunes {
			t.Fatalf("names of %d: %d characters, cap %d", nameLen, n, maxDigestRunes)
		}
		if !strings.Contains(out, "Merged without you: 12 code changes with no recorded review and 12 on protected paths.") {
			t.Fatalf("names of %d: the conclusion counts both groups:\n%s", nameLen, out)
		}
		if !strings.Contains(out, `Both lists are under "Merged".`) {
			// Only names no code host allows leave no room for a line of each list; then the
			// text is cut and the conclusion does not point at a list. Up to 200 characters
			// "Merged" fits by giving up lines, down to minMergeLines.
			if nameLen <= 200 || !strings.Contains(out, "…cut to fit") {
				t.Errorf("names of %d: the lists fit, so the conclusion says where they are:\n%s", nameLen, out)
			}
			cut++
			continue
		}
		if strings.Contains(out, "…cut to fit") {
			t.Errorf("names of %d: the text was cut, so the conclusion must not point at a list:\n%s", nameLen, out)
		}
		for _, title := range []string{"Protected paths (12):", "Code without a recorded independent review (12):"} {
			if !listedUnder(out, title) {
				t.Errorf("names of %d: the conclusion points at %q, which has no pull request under it:\n%s", nameLen, title, out)
			}
		}
	}
	if cut == 0 {
		t.Error("no case was long enough to cut the text, so the conclusion without its pointer was not checked")
	}
}

// twoLongRepositories is a day with 5 waiting tasks and 24 merges, 12 unreviewed and 12 on protected
// paths, in two repositories whose "owner/name" is nameLen characters long.
func twoLongRepositories(at time.Time, nameLen int) Digest {
	d := Digest{Summary: Summary{Since: at.Add(-24 * time.Hour), Until: at, Waiting: map[flow.State]int64{flow.Escalated: 5}}, Health: &Health{}}
	for i := range 5 {
		d.Summary.NeedsYou = append(d.Summary.NeedsYou, Waiting{Title: fmt.Sprintf("a task that waits for the owner with a title of some length %d", i), State: flow.Escalated})
	}
	for n := 1; n <= 24; n++ {
		repo := fmt.Sprintf("an-organization/%s-%d", strings.Repeat("r", nameLen-17), n%2)
		m := integration.MergedPR{Number: n, Title: fmt.Sprintf("fix(scope): a change with a title of ordinary length, about seventy characters %d", n), URL: fmt.Sprintf("https://github.com/%s/pull/%d", repo, n), MergedAt: at.Add(-time.Duration(n) * time.Minute)}
		if n > 12 {
			m.Reviewed, m.Labels = true, []string{gated}
		}
		d.Merges = append(d.Merges, m)
	}
	return d
}

// listedUnder is true when the line after title in out is a pull request.
func listedUnder(out, title string) bool {
	_, after, found := strings.Cut(out, title+"\n• ")
	line, _, _ := strings.Cut(after, "\n")
	return found && strings.Contains(line, "/pull/")
}

func TestManyRepositoriesGetTwoLinksAndACount(t *testing.T) {
	t.Parallel()
	var merges []integration.MergedPR
	for n := 1; n <= 40; n++ {
		merges = append(merges, integration.MergedPR{Number: n, Title: "docs: x", URL: fmt.Sprintf("https://github.com/example/repo-%d/pull/%d", n%5, n), Reviewed: true, Labels: []string{gated}})
	}
	out := Digest{Summary: Summary{Since: time.Now()}, Merges: merges}.mergedBlock(maxMergeLines)
	if n := strings.Count(out, "• all on protected paths: "); n != maxRepoLinks {
		t.Errorf("%d links to protected paths, want %d:\n%s", n, maxRepoLinks, out)
	}
	// 40 merges, 8 in each of 5 repositories, 8 listed: 32 are left, 7 in each of the two
	// repositories that get a link and 18 in the three that do not.
	if !strings.HasSuffix(out, "\n• 18 more in 3 other repositories") || strings.Count(out, "• 7 more: https://") != 2 || strings.Count(out, " more: https://") != maxRepoLinks {
		t.Errorf("past %d repositories the rest is one count:\n%s", maxRepoLinks, out)
	}
}

func TestTheHeadSaysWhichNumbersCoverAllTime(t *testing.T) {
	t.Parallel()
	out := Digest{Summary: Summary{Since: time.Now(), Finished: 4, Active: 2, Merged: 50, WithoutChanges: 40, Median: 90 * time.Minute}}.Format()
	if !strings.Contains(out, "The factory's own tasks: 4 finished, 2 in work. All time: 40 of 50 accepted without your changes, median time to merge 1.5 h.") {
		t.Errorf("the window's numbers and the all-time numbers are told apart:\n%s", out)
	}
}

// Regression tests for the second review of PR #365.

// "Merged" gives up lines, down to minMergeLines, when that lets the health detail, the counts of
// what changed and the plan show: with a long health detail the first fix kept eight pull request
// lines and lost "What changed", "Plan" and the detail of a red main.
func TestMergedGivesUpLinesForTheHealthDetailTheCountsAndThePlan(t *testing.T) {
	t.Parallel()
	for _, waiting := range []int{0, 3, 5} {
		d := busyNight()
		d.Health = &Health{MainFailed: []string{"example/app: test, lint"}, Now: d.Summary.Until}
		for i := range 5 {
			d.Health.Alerts = append(d.Health.Alerts, integration.Alert{Repo: "example/app", Number: i, Rule: "go/a-rule-with-a-name", Severity: "high", URL: fmt.Sprintf("https://github.com/example/app/security/code-scanning/%d", i)})
			d.Health.Idle = append(d.Health.Idle, integration.PRActivity{Repo: "example/app", Number: 300 + i, Title: "feat: a pull request that nobody touched for a while", URL: fmt.Sprintf("https://github.com/example/app/pull/%d", 300+i), Updated: d.Summary.Until.Add(-48 * time.Hour)})
		}
		for i := range waiting {
			d.Summary.NeedsYou = append(d.Summary.NeedsYou, Waiting{Title: fmt.Sprintf("a task that waits for the owner with a title of some length %d", i), State: flow.Escalated})
		}
		d.Summary.Waiting = map[flow.State]int64{flow.Escalated: int64(waiting)}
		out := d.Format()
		if n := len([]rune(out)); n > maxDigestRunes {
			t.Fatalf("%d waiting: %d characters, cap %d", waiting, n, maxDigestRunes)
		}
		for _, want := range []string{"*Factory health*\n• main is RED: example/app: test, lint", "*What changed*\n", "*Plan*\n", "Protected paths (53):\n• ", "Code without a recorded independent review (28):\n• "} {
			if !strings.Contains(out, want) {
				t.Errorf("%d waiting: lacks %q (%d characters):\n%s", waiting, want, len([]rune(out)), out)
			}
		}
	}
	// A day with room keeps all eight lines.
	if out := busyNight().Format(); strings.Count(out, "/pull/") != maxMergeLines {
		t.Errorf("nothing else to fit, so \"Merged\" keeps its %d lines:\n%s", maxMergeLines, out)
	}
}

// A revert takes back one merge, the latest before it with that title in the same repository.
func TestARevertTakesBackOneMergeBeforeIt(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	pr := func(minute int, repo, title string) integration.MergedPR {
		return integration.MergedPR{Number: minute, Title: title, URL: fmt.Sprintf("https://github.com/example/%s/pull/%d", repo, minute), MergedAt: at.Add(time.Duration(minute) * time.Minute)}
	}
	for name, c := range map[string]struct {
		merges []integration.MergedPR
		want   string
	}{
		"the change landed again after the revert": {
			[]integration.MergedPR{pr(1, "app", "feat: export"), pr(2, "app", `Revert "feat: export"`), pr(3, "app", "feat: export")},
			"new 1, reverted 1.",
		},
		"one of three equal titles is taken back": {
			[]integration.MergedPR{pr(1, "app", "chore: bump deps"), pr(2, "app", "chore: bump deps"), pr(3, "app", `Revert "chore: bump deps"`), pr(4, "app", "chore: bump deps")},
			"reverted 1, docs, tests and upkeep 2.",
		},
		"a revert of a revert lands the change again": {
			[]integration.MergedPR{pr(1, "app", "feat: export"), pr(2, "app", `Revert "feat: export"`), pr(3, "app", `Revert "Revert "feat: export""`)},
			"new 1.",
		},
		"a revert written as a commit type": {
			[]integration.MergedPR{pr(1, "app", "feat: export"), pr(2, "app", "revert: feat: export")},
			"reverted 1.",
		},
		"the code host's title with a tail": {
			[]integration.MergedPR{pr(1, "app", "feat: export"), pr(2, "app", `Revert "feat: export" (#1)`)},
			"reverted 1.",
		},
		"the code host's title with the commit type in front is one revert": {
			[]integration.MergedPR{pr(1, "app", "feat: export"), pr(2, "app", `revert: Revert "feat: export"`)},
			"reverted 1.",
		},
		"another repository keeps its change": {
			[]integration.MergedPR{pr(1, "app", "feat: export"), pr(2, "site", `Revert "feat: export"`)},
			"new 1, reverted 1.",
		},
		"a revert of something older than the window takes nothing back": {
			[]integration.MergedPR{pr(1, "app", `Revert "feat: export"`), pr(2, "app", "feat: export")},
			"new 1, reverted 1.",
		},
	} {
		if got := (Digest{Merges: c.merges}).changedCounts(); got != "*What changed*\n"+c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

// The links past the first go to the repositories with the most merges left out.
func TestRepositoryLinksGoToTheBusiestRepositories(t *testing.T) {
	t.Parallel()
	var merges []integration.MergedPR
	for n, repo := range []string{"a", "b", "b", "b", "c", "c", "c", "c", "d", "d"} {
		for i := range 5 {
			merges = append(merges, integration.MergedPR{Number: n*10 + i, Title: "docs: x", URL: fmt.Sprintf("https://github.com/example/%s/pull/%d", repo, n*10+i), Reviewed: true})
		}
	}
	out := Digest{Summary: Summary{Since: time.Now()}, Merges: merges}.mergedBlock(0)
	if !strings.Contains(out, "• 20 more: https://github.com/example/c/pulls") || !strings.Contains(out, "• 15 more: https://github.com/example/b/pulls") || !strings.HasSuffix(out, "\n• 15 more in 2 other repositories") {
		t.Errorf("want links for c (20) and b (15), and one count for a and d:\n%s", out)
	}
}

// Regression tests for the third review of PR #365.

// "Merged" gives up lines only for a block that then shows. A health detail too long for any
// budget used to cost six pull request lines and still not show; and fewer lines are not always
// a shorter block, since each dropped line of a repository becomes a link.
func TestMergedKeepsItsLinesWhenGivingThemUpPlacesNothing(t *testing.T) {
	t.Parallel()
	d := busyNight()
	d.Health = &Health{AlertsErr: errors.New(strings.Repeat("the alerts of a repository could not be read; ", 70)), Now: d.Summary.Until}
	if n := runes(d.Health.block()); n < maxDigestRunes {
		t.Fatalf("the health detail must fit at no budget, it is %d characters", n)
	}
	out := d.Format()
	if strings.Contains(out, "*Factory health*") || !strings.Contains(out, d.mergedBlock(maxMergeLines)) {
		t.Errorf("the health detail fits at no budget, so \"Merged\" keeps its %d lines:\n%s", maxMergeLines, out)
	}
	for _, want := range []string{"*What changed*\n", "*Plan*\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
}

// The health detail counts for more than the counts and the plan together, as in Digest.optional.
func TestAnEarlierEssentialBlockOutweighsTheLaterOnes(t *testing.T) {
	t.Parallel()
	essential := []optionalBlock{{blockHealth, "h"}, {blockChanged, "c"}, {blockPlan, ""}}
	var health, counts [blockHealth + 1]string
	health[blockHealth], counts[blockChanged] = "h", "c"
	h, all := essentialsIn(health[:], essential)
	c, _ := essentialsIn(counts[:], essential)
	if h <= c || all != h+c {
		t.Errorf("health alone scores %d, the counts alone %d, both %d: want health above the counts, and a block without text not counted", h, c, all)
	}
}

// A revert of a revert is listed as the change it lands again, by that change's title.
func TestARevertOfARevertIsListedAsTheChange(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	d := Digest{Summary: Summary{Since: at}, Merges: []integration.MergedPR{
		{Number: 3, Title: `Revert "Revert "feat(export): a file per tenant""`, URL: "https://github.com/example/app/pull/3", MergedAt: at.Add(time.Hour)},
	}}
	if out := d.changedBlock(); !strings.Contains(out, "New (1):\n• export: a file per tenant") || strings.Contains(out, "Revert") {
		t.Errorf("want the change under \"New\" by its own title:\n%s", out)
	}
}
