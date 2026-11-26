// Package summary builds the morning summary and the `/ns status` answer from the book
// (design sections 8 and 15). It is plain SQL and text: no model writes it, so the numbers
// are always the book's.
package summary

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/flow"
)

// waitingStates hold a task until the owner acts.
var waitingStates = []flow.State{flow.Spec, flow.DesignReview, flow.HumanReview, flow.Ready, flow.Escalated}

// activeStates are the ones where the factory is working.
var activeStates = []flow.State{
	flow.Grilling, flow.DocChecks, flow.Queued, flow.Building, flow.Checks, flow.Review,
	flow.Preview, flow.Acceptance, flow.Merging,
}

// Summary is what the message says.
type Summary struct {
	Since    time.Time
	Until    time.Time // end of the window; zero when unknown
	Finished int64     // tasks that ended or were handed off in the window
	Active   int64
	Waiting  map[flow.State]int64
	// NeedsYou names the tasks that wait for the owner, oldest first.
	NeedsYou []Waiting
	// Merged, WithoutChanges and Median are over all merged tasks of the tenant.
	Merged         int64
	WithoutChanges int64
	Median         time.Duration
	// LimitHits counts jobs per provider that stopped on a model limit in the window;
	// FallbackJobs counts jobs that finished on a fallback provider (ADR 0012).
	LimitHits    map[string]int64
	FallbackJobs int64
}

// considered are the states of tasks that can replace a waiting one: a later task with the same
// title that ended or is in work.
var considered = []flow.State{flow.Released, flow.MergedByTeam, flow.HandedOff, flow.Queued, flow.Building, flow.Checks, flow.Review, flow.Merging}

// needsYou keeps the tasks that wait for the owner, oldest first, and drops one that a later task
// with the same title replaced (a task that escalated and was redone under a new id).
func needsYou(all []bookdb.BookTask) []Waiting {
	slices.SortFunc(all, func(a, b bookdb.BookTask) int { return int(a.ID - b.ID) })
	var out []Waiting
	for i, t := range all {
		if !slices.Contains(waitingStates, flow.State(t.State)) {
			continue
		}
		if slices.ContainsFunc(all[i+1:], func(l bookdb.BookTask) bool { return l.Title == t.Title }) {
			continue
		}
		out = append(out, Waiting{Title: t.Title, State: flow.State(t.State)})
	}
	return out
}

// Waiting is one task that waits for the owner.
type Waiting struct {
	Title string
	State flow.State
}

// Build reads the numbers for a tenant, for the window from since to until (the moment the
// message is made, so that it can name the window). fallback names
// the providers marked `tier: fallback`, so their finished jobs can be counted.
func Build(ctx context.Context, pool *pgxpool.Pool, tenant string, since, until time.Time, fallback ...string) (Summary, error) {
	q := bookdb.New(pool)
	counts, err := q.CountTasksByState(ctx, tenant)
	if err != nil {
		return Summary{}, fmt.Errorf("summary: %w", err)
	}
	s := Summary{Since: since, Until: until, Waiting: map[flow.State]int64{}}
	for _, c := range counts {
		st := flow.State(c.State)
		switch {
		case slices.Contains(waitingStates, st):
			s.Waiting[st] = c.Tasks
		case slices.Contains(activeStates, st):
			s.Active += c.Tasks
		}
	}
	all, err := q.ListActiveTasks(ctx, bookdb.ListActiveTasksParams{Tenant: tenant, States: stateNames(append(slices.Clone(waitingStates), considered...))})
	if err != nil {
		return Summary{}, fmt.Errorf("summary: %w", err)
	}
	s.NeedsYou = needsYou(all)
	// The counts follow the list: a task that a later one replaced no longer waits for the owner.
	s.Waiting = map[flow.State]int64{}
	for _, w := range s.NeedsYou {
		s.Waiting[w.State]++
	}
	if s.Finished, err = q.CountFinishedSince(ctx, bookdb.CountFinishedSinceParams{Tenant: tenant, Since: pgtype.Timestamptz{Time: since, Valid: true}}); err != nil {
		return Summary{}, fmt.Errorf("summary: %w", err)
	}
	m, err := q.MergeStats(ctx, tenant)
	if err != nil {
		return Summary{}, fmt.Errorf("summary: %w", err)
	}
	s.Merged, s.WithoutChanges = m.Merged, m.WithoutChanges
	s.Median = time.Duration(m.MedianSeconds * float64(time.Second))
	jobs, err := q.ProviderJobsSince(ctx, bookdb.ProviderJobsSinceParams{Tenant: tenant, Since: pgtype.Timestamptz{Time: since, Valid: true}})
	if err != nil {
		return Summary{}, fmt.Errorf("summary: %w", err)
	}
	s.LimitHits = map[string]int64{}
	for _, j := range jobs {
		switch {
		case j.Limited:
			s.LimitHits[j.Provider] += j.Jobs
		case slices.Contains(fallback, j.Provider):
			s.FallbackJobs += j.Jobs
		}
	}
	return s, nil
}

// Format renders the status answer: the head, then what needs the owner.
func (s Summary) Format() string {
	return join(s.head("Status", ""), s.needsYouBlock())
}

// head is the block that always comes first: the window, the counts and what needs the owner,
// each on a line of its own. extra is a line the caller adds after the counts.
func (s Summary) head(title, extra string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*%s* · %s\n", title, s.window())
	fmt.Fprintf(&b, "%d finished, %d in work. ", s.Finished, s.Active)
	if n := s.waitingTotal(); n == 0 {
		b.WriteString("Nothing waits for you.\n")
	} else {
		fmt.Fprintf(&b, "%d need%s you.\n", n, plural(n, "", "s"))
	}
	if extra != "" {
		b.WriteString(extra + "\n")
	}
	if s.Merged == 0 {
		b.WriteString("No merged tasks yet, so no merge numbers.")
	} else {
		fmt.Fprintf(&b, "Accepted without your changes: %d of %d (%d%%). Median time to merge: %s.",
			s.WithoutChanges, s.Merged, 100*s.WithoutChanges/s.Merged, duration(s.Median))
	}
	if line := s.limitLine(); line != "" {
		b.WriteString("\n" + line)
	}
	return b.String()
}

// window says how far back the numbers reach, in the words of the owner ("last 24 h"), with the
// start as a date, so a window that is not what was asked for is visible at once.
func (s Summary) window() string {
	start := s.Since.Format("02 Jan 15:04 MST")
	if s.Until.IsZero() || !s.Until.After(s.Since) {
		return "since " + start
	}
	d := s.Until.Sub(s.Since).Round(time.Minute)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("last %d min (since %s)", int(d.Minutes()), start)
	case d%time.Hour == 0:
		return fmt.Sprintf("last %d h (since %s)", int(d.Hours()), start)
	default:
		return fmt.Sprintf("last %.1f h (since %s)", d.Hours(), start)
	}
}

func (s Summary) waitingTotal() int64 {
	var n int64
	for _, st := range waitingStates {
		n += s.Waiting[st]
	}
	return n
}

// needsYouPhrase says in plain words what the owner has to do for a task in each waiting state.
var needsYouPhrase = map[flow.State]string{
	flow.Spec:         "approve the spec",
	flow.DesignReview: "review the design",
	flow.HumanReview:  "review the pull request",
	flow.Ready:        "ready to merge",
	flow.Escalated:    "stuck, decide how to go on",
}

// maxNeedsYou caps the list of waiting tasks; the rest is a count.
const maxNeedsYou = 5

// needsYouBlock lists what waits for the owner, empty when nothing does. Without task names (a
// summary built from counts) it lists the states.
func (s Summary) needsYouBlock() string {
	total := s.waitingTotal()
	if total == 0 {
		return ""
	}
	lines := []string{fmt.Sprintf("*Needs you* (%d)", total)}
	for i, w := range s.NeedsYou {
		if i == maxNeedsYou {
			lines = append(lines, fmt.Sprintf("• %d more", len(s.NeedsYou)-maxNeedsYou))
			break
		}
		lines = append(lines, fmt.Sprintf("• %s: %s", short(w.Title, maxTitle), needsYouPhrase[w.State]))
	}
	if len(s.NeedsYou) == 0 {
		for _, st := range waitingStates {
			if n := s.Waiting[st]; n > 0 {
				lines = append(lines, fmt.Sprintf("• %d: %s", n, needsYouPhrase[st]))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// maxTitle is the longest task or pull request title shown on a line, in characters.
const maxTitle = 60

// sep splits a message into blocks. A chat adapter that can draw a divider turns it into one;
// everywhere else it reads as a rule.
const sep = "\n---\n"

func join(blocks ...string) string {
	var out []string
	for _, b := range blocks {
		if b != "" {
			out = append(out, b)
		}
	}
	return strings.Join(out, sep)
}

// short cuts s to n characters at a word boundary when it can.
func short(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

func plural(n int64, many, one string) string {
	if n == 1 {
		return one
	}
	return many
}

func stateNames(in []flow.State) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = string(s)
	}
	return out
}

func duration(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%.1f h", d.Hours())
	default:
		return fmt.Sprintf("%.1f days", d.Hours()/24)
	}
}

// limitLine says what model limits did in the window; empty when they did nothing.
func (s Summary) limitLine() string {
	if len(s.LimitHits) == 0 && s.FallbackJobs == 0 {
		return ""
	}
	var parts []string
	for _, p := range slices.Sorted(maps.Keys(s.LimitHits)) {
		parts = append(parts, fmt.Sprintf("%s %d", p, s.LimitHits[p]))
	}
	line := "Model limits:"
	if len(parts) > 0 {
		line += " hit by " + strings.Join(parts, ", ") + ";"
	}
	if s.FallbackJobs > 0 {
		line += fmt.Sprintf(" %d jobs ran on a fallback provider;", s.FallbackJobs)
	}
	return strings.TrimSuffix(line, ";") + "."
}
