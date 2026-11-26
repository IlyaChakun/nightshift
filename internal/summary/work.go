package summary

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// Limits of "What changed": how many of the newest pull requests a line names, and how long each
// may be, in characters.
const (
	maxChangeItems = 3
	maxChangeWhat  = 80
)

// maxWhat is the longest "what was done" shown for a pull request, in characters.
const maxWhat = 80

var (
	conventional = regexp.MustCompile(`^(\w+)(?:\(([^)]*)\))?(!?):\s*(.*)$`)
	// revertTitle is a revert by its title: "revert: x", "revert(scope): x" or the code host's
	// own `Revert "x"`. Group 1 or 2 is the title it takes back. A title that only starts with the
	// word ("Revert button on the card") is not one.
	revertTitle = regexp.MustCompile(`(?i)^revert(?:(?:\([^)]*\))?!?:\s*(.*)|\s+"(.*)")$`)
)

// commitTypes are the conventional-commit types whose prefix is dropped from a title and names its
// area; any other word before a colon ("Revert:", "WIP:") is part of what was done.
var commitTypes = map[string]bool{
	"feat": true, "fix": true, "docs": true, "chore": true, "test": true, "ci": true,
	"refactor": true, "perf": true, "build": true, "style": true, "deps": true,
}

// parseTitle splits "type(scope): rest"; ok is false when the type is not a known one.
func parseTitle(title string) (typ, scope, rest string, ok bool) {
	g := conventional.FindStringSubmatch(strings.TrimSpace(title))
	if g == nil || !commitTypes[strings.ToLower(g[1])] {
		return "", "", title, false
	}
	return strings.ToLower(g[1]), strings.ToLower(g[2]), g[4], true
}

// breaking is true for a title marked as a breaking change ("chore!: drop the old flag").
func breaking(title string) bool {
	g := conventional.FindStringSubmatch(strings.TrimSpace(title))
	return g != nil && g[3] == "!"
}

// what says in one line what a merge did: its title without the conventional prefix. The title
// states the outcome; the "What and why" paragraph is context and often starts with the plan.
func what(m integration.MergedPR) string { return whatN(m, maxWhat) }

func whatN(m integration.MergedPR, n int) string {
	text := m.Title
	if _, _, rest, ok := parseTitle(text); ok && rest != "" {
		text = rest
	}
	return short(text, n)
}

// whatScoped is what a merge did with the part it touched in front ("deploy: a release rolls
// back"), cut to n characters. A title without a scope is its text alone.
func whatScoped(m integration.MergedPR, n int) string {
	if _, scope, rest, ok := parseTitle(m.Title); ok && scope != "" && rest != "" {
		return short(scope+": "+rest, n)
	}
	return whatN(m, n)
}

// change is the kind of result a merge is for the owner.
type change int

const (
	changeNew change = iota
	changeFixed
	changeReverted
	changeOther
	changeUpkeep
)

// changeTitle heads the line of each kind that names its pull requests; upkeep is only counted.
var changeTitle = [...]string{changeNew: "New", changeFixed: "Fixed", changeReverted: "Reverted", changeOther: "Other"}

// revertOf says whether a title is a revert, and the title it takes back ("" when it names none).
func revertOf(title string) (taken string, ok bool) {
	g := revertTitle.FindStringSubmatch(strings.TrimSpace(title))
	if g == nil {
		return "", false
	}
	return strings.TrimSpace(g[1] + g[2]), true
}

// changeOf sorts a merge by its title: a revert, a feature, a fix, upkeep (docs, tests, chores and
// the like), or a title with no known type. Upkeep that breaks something is named, not counted. A
// revert of a revert lands the change again, so it is what that change is.
func changeOf(m integration.MergedPR) change {
	if taken, ok := revertOf(m.Title); ok {
		if again, twice := revertOf(taken); twice {
			return changeOf(integration.MergedPR{Title: again})
		}
		return changeReverted
	}
	switch typ, _, _, ok := parseTitle(m.Title); {
	case !ok:
		return changeOther
	case typ == "feat":
		return changeNew
	case typ == "fix":
		return changeFixed
	case breaking(m.Title):
		return changeOther
	}
	return changeUpkeep
}

// changes sorts the merges by kind, each kind newest first. A revert takes back one merge: the
// latest one before it in the same repository whose title it names. That merge is left out, since
// it is not there any more; a change that landed again after the revert stays.
func (d Digest) changes() [changeUpkeep + 1][]integration.MergedPR {
	ms := slices.Clone(d.Merges)
	slices.SortStableFunc(ms, func(x, y integration.MergedPR) int { return x.MergedAt.Compare(y.MergedAt) })
	gone := make([]bool, len(ms))
	for i, r := range ms {
		taken, ok := revertOf(r.Title)
		if !ok || taken == "" {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if !gone[j] && strings.TrimSpace(ms[j].Title) == taken && repoOf(ms[j].URL) == repoOf(r.URL) {
				gone[j] = true
				break
			}
		}
	}
	var by [changeUpkeep + 1][]integration.MergedPR
	for i, m := range slices.Backward(ms) {
		if !gone[i] {
			c := changeOf(m)
			by[c] = append(by[c], m)
		}
	}
	return by
}

// changedBlock says what came out of the merges in the owner's terms: what is new, what was
// fixed and what was reverted, each with its newest titles a line each, and one count for the upkeep. Areas
// of the code and pull request counts per area say what was busy, not what changed.
func (d Digest) changedBlock() string {
	if len(d.Merges) == 0 {
		return ""
	}
	by := d.changes()
	lines := []string{"*What changed*"}
	for c := range changeUpkeep {
		ms := by[c]
		if len(ms) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s (%d):", changeTitle[c], len(ms)))
		for _, m := range ms[:min(len(ms), maxChangeItems)] {
			lines = append(lines, "• "+whatScoped(m, maxChangeWhat))
		}
		if rest := len(ms) - maxChangeItems; rest > 0 {
			lines = append(lines, fmt.Sprintf("• %d more", rest))
		}
	}
	if n := len(by[changeUpkeep]); n > 0 {
		lines = append(lines, fmt.Sprintf("Docs, tests and upkeep: %d", n))
	}
	return strings.Join(lines, "\n")
}

// changedCounts is "What changed" on one line, for a message with no room for the titles.
func (d Digest) changedCounts() string {
	if len(d.Merges) == 0 {
		return ""
	}
	by := d.changes()
	var parts []string
	for c := range changeUpkeep {
		if n := len(by[c]); n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", strings.ToLower(changeTitle[c]), n))
		}
	}
	if n := len(by[changeUpkeep]); n > 0 {
		parts = append(parts, fmt.Sprintf("docs, tests and upkeep %d", n))
	}
	return "*What changed*\n" + strings.Join(parts, ", ") + "."
}

// conclusion closes the message with the one thing the blocks above do not say: how much went in
// without the owner, and where it is listed (pointer is false when the lists may have been cut).
// What waits and what is wrong are in the head, so they are not repeated; a day with nothing to
// do says so, and a day whose work is all in the head gets no conclusion.
func (d Digest) conclusion(pointer bool) string {
	var n [groupOther + 1]int64
	for _, m := range d.Merges {
		n[d.group(m)]++
	}
	unreviewed, protected := n[groupUnreviewed], n[groupGated]
	var line, where string
	switch {
	case d.MergeErr != nil: // the head says the list is incomplete
	case unreviewed > 0 && protected > 0:
		line = fmt.Sprintf("Merged without you: %d code change%s with no recorded review and %d on protected paths.", unreviewed, plural(unreviewed, "s", ""), protected)
		where = " Both lists are under \"Merged\"."
	case unreviewed > 0:
		line = fmt.Sprintf("Merged without you: %d code change%s with no recorded review.", unreviewed, plural(unreviewed, "s", ""))
		where = " They are listed under \"Merged\"."
	case protected > 0:
		line = fmt.Sprintf("Merged without you: %d change%s on protected paths.", protected, plural(protected, "s", ""))
		where = " They are listed under \"Merged\"."
	}
	switch {
	case line != "" && pointer:
		line += where
	case line != "":
	case d.Summary.waitingTotal() == 0 && len(d.Health.problems()) == 0 && d.MergeErr == nil:
		line = "Nothing to do today."
	default:
		return ""
	}
	return "*Conclusion*\n• " + line
}

// problems lists what is wrong with the factory's health: main, alerts, idle pull requests, or a
// signal that could not be read.
func (h *Health) problems() []string {
	if h == nil {
		return nil
	}
	var bad []string
	if h.MainErr != nil {
		bad = append(bad, "main is unknown")
	}
	if len(h.MainFailed) > 0 {
		bad = append(bad, "main is red")
	}
	if h.AlertsErr == nil && len(h.Alerts) > 0 {
		bad = append(bad, fmt.Sprintf("%d open code-scanning alerts", len(h.Alerts)))
	}
	if h.PRsErr == nil && len(h.Idle) > 0 {
		bad = append(bad, fmt.Sprintf("%d idle pull requests", len(h.Idle)))
	}
	if h.AlertsErr != nil || h.PRsErr != nil {
		bad = append(bad, "some health checks could not be read")
	}
	return bad
}

// verdict is one line on the health signals: what is wrong, or that nothing is.
func (h *Health) verdict() string {
	if h == nil {
		return ""
	}
	if bad := h.problems(); len(bad) > 0 {
		return "Look at: " + strings.Join(bad, ", ") + "."
	}
	return "Main is green, no open alerts, nothing idle."
}

// Limits of the "Plan" block: how many plan files it names, and how long a name may be.
const (
	maxPlanFiles = 6
	maxPlanName  = 60
)

var phaseNumber = regexp.MustCompile(`^Phase (\d+)`)

// planOrder puts the phases first, by number, and every other plan file after them, by name: the
// order the plan itself is read in.
func planOrder(a, b integration.PlanProgress) int {
	num := func(p integration.PlanProgress) int {
		if m := phaseNumber.FindStringSubmatch(p.Name); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				return n
			}
		}
		return math.MaxInt
	}
	return cmp.Or(cmp.Compare(num(a), num(b)), strings.Compare(a.Name, b.Name))
}

// planBlock says how far the open parts of the plan have come, one named file a line ("Phase 5 —
// Lead: 14 of 20 done"), in plan order. A finished file and a file with no items are left out;
// nothing to say gives no block. When more files are open than fit, the ones with the most open
// items stay, so a nearly finished phase does not hide the one with the work.
func planBlock(plan []integration.PlanProgress) string {
	var open []integration.PlanProgress
	for _, p := range plan {
		if p.Total > 0 && p.Done < p.Total {
			open = append(open, p)
		}
	}
	if len(open) == 0 {
		return ""
	}
	rest := int64(max(0, len(open)-maxPlanFiles))
	if rest > 0 {
		slices.SortStableFunc(open, func(a, b integration.PlanProgress) int {
			return cmp.Or(cmp.Compare(b.Total-b.Done, a.Total-a.Done), planOrder(a, b))
		})
		open = open[:maxPlanFiles]
	}
	slices.SortStableFunc(open, planOrder)
	lines := []string{"*Plan*"}
	for _, p := range open {
		lines = append(lines, fmt.Sprintf("• %s: %d of %d done", short(p.Name, maxPlanName), p.Done, p.Total))
	}
	if rest > 0 {
		lines = append(lines, fmt.Sprintf("• %d more plan file%s with open items", rest, plural(rest, "s", "")))
	}
	return strings.Join(lines, "\n")
}

// BuildPlan reads the plan progress of each repository (owner/name). A repository whose plan
// cannot be read adds nothing: the line is a help, and a missing one must not hold the digest.
func BuildPlan(ctx context.Context, src integration.PlanLog, repos []string) []integration.PlanProgress {
	if src == nil {
		return nil
	}
	var out []integration.PlanProgress
	for _, r := range repos {
		p, err := src.PlanProgress(ctx, r)
		if err != nil {
			continue
		}
		out = append(out, p...)
	}
	return out
}
