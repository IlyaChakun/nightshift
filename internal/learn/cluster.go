// Package learn holds the deterministic half of the nightly lessons loop (design section 10):
// it groups findings into recurring problems and picks which of them are worth a fix this
// night. Finding the root cause and writing the fix is the model's job; nothing here calls one.
package learn

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
)

// Finding is the part of a book finding that clustering reads.
type Finding struct {
	TaskID        int64
	RuleID        string
	Category      string
	LayerCaught   string
	LayerExpected string // empty when nobody said which layer should have caught it
	Message       string
}

// Cluster is one recurring problem.
type Cluster struct {
	Key     string
	Count   int      // findings in the cluster
	Tasks   int      // distinct tasks they came from
	Layers  []string // layers that caught them, sorted
	Late    bool     // some finding was caught later than the layer that should have caught it
	Samples []string // up to three distinct messages, for the model that looks for the cause
}

// layerOrder is the order of the layers of design section 8; a later layer is dearer.
var layerOrder = []string{"L0", "L1", "L2", "L3", "L4", "L5", "L6", "prod"}

func later(caught, expected string) bool {
	c, e := slices.Index(layerOrder, caught), slices.Index(layerOrder, expected)
	return c >= 0 && e >= 0 && c > e
}

// key names the problem a finding belongs to. A finding with a rule is grouped by the rule;
// one without (`new`) by its category and the first words of its message, so the same
// complaint worded the same way lands together.
func key(f Finding) string {
	if f.RuleID != "" && f.RuleID != "new" {
		return f.RuleID
	}
	words := strings.FieldsFunc(strings.ToLower(f.Message), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) > 4 {
		words = words[:4]
	}
	return "new:" + f.Category + ":" + strings.Join(words, "-")
}

// Recurring groups findings and keeps the clusters seen in at least minTasks distinct tasks,
// a single task's repeats being a Builder loop and not a lesson. Clusters in `rejected` (a
// rule the owner turned down or retired) are left out. The result is worst first: late
// catches, then more tasks, then more findings, then the key; at most limit clusters come
// back (a night opens at most three PRs).
func Recurring(fs []Finding, minTasks, limit int, rejected map[string]bool) []Cluster {
	type acc struct {
		c     Cluster
		tasks map[int64]bool
		seen  map[string]bool
		layer map[string]bool
	}
	by := map[string]*acc{}
	for _, f := range fs {
		k := key(f)
		if rejected[k] {
			continue
		}
		a := by[k]
		if a == nil {
			a = &acc{c: Cluster{Key: k}, tasks: map[int64]bool{}, seen: map[string]bool{}, layer: map[string]bool{}}
			by[k] = a
		}
		a.c.Count++
		a.tasks[f.TaskID] = true
		a.layer[f.LayerCaught] = true
		a.c.Late = a.c.Late || later(f.LayerCaught, f.LayerExpected)
		if len(a.c.Samples) < 3 && !a.seen[f.Message] {
			a.seen[f.Message] = true
			a.c.Samples = append(a.c.Samples, f.Message)
		}
	}
	var out []Cluster
	for _, a := range by {
		a.c.Tasks = len(a.tasks)
		if a.c.Tasks < minTasks {
			continue
		}
		for l := range a.layer {
			a.c.Layers = append(a.c.Layers, l)
		}
		slices.Sort(a.c.Layers)
		out = append(out, a.c)
	}
	slices.SortFunc(out, func(x, y Cluster) int {
		if x.Late != y.Late {
			if x.Late {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(y.Tasks, x.Tasks), cmp.Compare(y.Count, x.Count), cmp.Compare(x.Key, y.Key))
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
