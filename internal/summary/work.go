package summary

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// maxAreas caps the "Work and results" list; the rest is a count.
const maxAreas = 5

// maxWhat is the longest "what was done" shown for a pull request, in characters.
const maxWhat = 80

// maxAreaItems is how many of the newest pull requests an area line names, each cut to maxAreaWhat.
const (
	maxAreaItems = 3
	maxAreaWhat  = 50
)

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

// maxArea is the longest area name shown, in characters.
const maxArea = 20

// typeArea names the area of a pull request that has a type and no scope.
var typeArea = map[string]string{
	"docs": "docs", "chore": "chores", "test": "tests", "ci": "ci", "fix": "fixes", "feat": "features", "deps": "dependencies",
}

// area says what part of the factory a merge touched: the scope of a conventional title
// ("dispatch"), else its type ("docs"), else "other".
func area(m integration.MergedPR) string {
	typ, scope, _, ok := parseTitle(m.Title)
	switch {
	case !ok:
		return "other"
	case scope != "":
		return short(scope, maxArea)
	case typeArea[typ] != "":
		return typeArea[typ]
	}
	return "other"
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

// workBlock groups every merge by area, biggest area first, and says what the newest pull request
// of each did. It is the answer to "what did we work on and what came out of it".
func (d Digest) workBlock() string {
	if len(d.Merges) == 0 {
		return ""
	}
	byArea := map[string][]integration.MergedPR{}
	for _, m := range d.Merges {
		a := area(m)
		byArea[a] = append(byArea[a], m)
	}
	names := slices.SortedFunc(maps.Keys(byArea), func(a, b string) int {
		return cmp.Or(len(byArea[b])-len(byArea[a]), strings.Compare(a, b))
	})
	lines := []string{"*Work and results*"}
	for i, a := range names {
		if i == maxAreas {
			rest := 0
			for _, n := range names[i:] {
				rest += len(byArea[n])
			}
			lines = append(lines, fmt.Sprintf("• %d more in %d other area%s", rest, len(names)-i, plural(int64(len(names)-i), "s", "")))
			break
		}
		ms := byArea[a]
		slices.SortStableFunc(ms, func(x, y integration.MergedPR) int { return y.MergedAt.Compare(x.MergedAt) })
		var done []string
		for _, m := range ms[:min(len(ms), maxAreaItems)] {
			done = append(done, whatN(m, maxAreaWhat))
		}
		lines = append(lines, fmt.Sprintf("• %s (%d): %s", a, len(ms), strings.Join(done, "; ")))
	}
	return strings.Join(lines, "\n")
}

// conclusion closes the message with what to do about it: the merges that went in without the
// owner, what waits, and the state of main and the alerts.
func (d Digest) conclusion() string {
	var n [4]int
	for _, m := range d.Merges {
		n[d.group(m)]++
	}
	lines := []string{"*Conclusion*"}
	if d.MergeErr != nil {
		lines = append(lines, "• The merge list is incomplete, so what merged without you is not fully known.")
	} else if n[groupGated]+n[groupUnreviewed] > 0 {
		lines = append(lines, fmt.Sprintf("• %d protected-path and %d unreviewed code merges went in without you: skim them above.", n[groupGated], n[groupUnreviewed]))
	} else {
		lines = append(lines, "• No protected-path or unreviewed code merged.")
	}
	if w := d.Summary.waitingTotal(); w > 0 {
		lines = append(lines, fmt.Sprintf("• %d task%s wait%s for you.", w, plural(w, "s", ""), plural(w, "", "s")))
	} else {
		lines = append(lines, "• Nothing waits for you.")
	}
	if l := d.Health.verdict(); l != "" {
		lines = append(lines, "• "+l)
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
