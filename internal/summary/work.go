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
	conventional = regexp.MustCompile(`^(\w+)(?:\(([^)]*)\))?!?:\s*(.*)$`)
)

// commitTypes are the conventional-commit types whose prefix is dropped from a title and names its
// area; any other word before a colon ("Revert:", "WIP:") is part of what was done.
var commitTypes = map[string]bool{
	"feat": true, "fix": true, "docs": true, "chore": true, "test": true, "ci": true,
	"refactor": true, "perf": true, "build": true, "style": true, "deps": true,
}

// parseTitle splits "type(scope): rest"; ok is false when the type is not a known one.
func parseTitle(title string) (typ, scope, rest string, ok bool) {
	g := conventional.FindStringSubmatch(title)
	if g == nil || !commitTypes[strings.ToLower(g[1])] {
		return "", "", title, false
	}
	return strings.ToLower(g[1]), strings.ToLower(g[2]), g[3], true
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

// changeOf sorts a merge by its title: a revert, a feature, a fix, upkeep (docs, tests, chores and
// the like), or a title with no known type.
func changeOf(m integration.MergedPR) change {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Title)), "revert") {
		return changeReverted
	}
	switch typ, _, _, ok := parseTitle(m.Title); {
	case !ok:
		return changeOther
	case typ == "feat":
		return changeNew
	case typ == "fix":
		return changeFixed
	}
	return changeUpkeep
}

// changedBlock says what came out of the merges in the owner's terms: what is new, what was
// fixed and what was reverted, each with its newest titles a line each, and one count for the upkeep. Areas
// of the code and pull request counts per area say what was busy, not what changed.
func (d Digest) changedBlock() string {
	if len(d.Merges) == 0 {
		return ""
	}
	var by [changeUpkeep + 1][]integration.MergedPR
	for _, m := range d.Merges {
		c := changeOf(m)
		by[c] = append(by[c], m)
	}
	lines := []string{"*What changed*"}
	for c := range changeUpkeep {
		ms := by[c]
		if len(ms) == 0 {
			continue
		}
		slices.SortStableFunc(ms, func(x, y integration.MergedPR) int { return y.MergedAt.Compare(x.MergedAt) })
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

// conclusion closes the message with what to do about it and nothing else: what waits, which
// merges went in without the owner, and what is wrong with the factory. The head already says
// that nothing waits and that main is green, so a quiet day gets one line here.
func (d Digest) conclusion() string {
	var n [4]int
	for _, m := range d.Merges {
		n[d.group(m)]++
	}
	lines := []string{"*Conclusion*"}
	if w := d.Summary.waitingTotal(); w > 0 {
		lines = append(lines, fmt.Sprintf("• Start with \"Needs you\": %d task%s wait%s for you.", w, plural(w, "s", ""), plural(w, "", "s")))
	}
	unreviewed, protected := int64(n[groupUnreviewed]), int64(n[groupGated])
	switch {
	case d.MergeErr != nil:
		lines = append(lines, "• The merge list is incomplete, so what merged without you is not fully known.")
	case unreviewed > 0 && protected > 0:
		lines = append(lines, fmt.Sprintf("• Merged without you: %d code change%s with no recorded review and %d on protected paths. Both lists are under \"Merged\".",
			unreviewed, plural(unreviewed, "s", ""), protected))
	case unreviewed > 0:
		lines = append(lines, fmt.Sprintf("• Merged without you: %d code change%s with no recorded review, listed under \"Merged\".", unreviewed, plural(unreviewed, "s", "")))
	case protected > 0:
		lines = append(lines, fmt.Sprintf("• Merged without you: %d change%s on protected paths, listed under \"Merged\".", protected, plural(protected, "s", "")))
	}
	if len(d.Health.problems()) > 0 {
		lines = append(lines, "• "+d.Health.verdict())
	}
	if len(lines) == 1 {
		lines = append(lines, "• Nothing to do today.")
	}
	return strings.Join(lines, "\n")
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
	maxPlanFiles = 4
	maxPlanName  = 60
)

var phaseNumber = regexp.MustCompile(`^Phase (\d+)\b`)

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
// Lead: 14 of 20 done"). A finished file and a file with no items are left out; nothing to say
// gives no block.
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
	slices.SortStableFunc(open, planOrder)
	lines := []string{"*Plan*"}
	for i, p := range open {
		if i == maxPlanFiles {
			rest := int64(len(open) - i)
			lines = append(lines, fmt.Sprintf("• %d more plan file%s with open items", rest, plural(rest, "s", "")))
			break
		}
		lines = append(lines, fmt.Sprintf("• %s: %d of %d done", short(p.Name, maxPlanName), p.Done, p.Total))
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
