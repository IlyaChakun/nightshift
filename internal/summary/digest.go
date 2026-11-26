package summary

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// Decision is one move of a task that was a decision, with who took it.
type Decision struct {
	TaskID int64
	Title  string
	Actor  string
	Event  string
	From   string
	To     string
	At     time.Time
}

// Decisions reads the decisions of the window from the book.
func Decisions(ctx context.Context, pool *pgxpool.Pool, tenant string, since time.Time) ([]Decision, error) {
	rows, err := bookdb.New(pool).DecisionsSince(ctx, bookdb.DecisionsSinceParams{Tenant: tenant, Since: pgtype.Timestamptz{Time: since, Valid: true}})
	if err != nil {
		return nil, fmt.Errorf("summary: decisions: %w", err)
	}
	out := make([]Decision, 0, len(rows))
	for _, r := range rows {
		out = append(out, Decision{
			TaskID: r.TaskID.Int64, Title: r.Title, Actor: r.Actor, Event: r.Event,
			From: r.FromState, To: r.ToState, At: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// Digest is the morning summary plus everything merged and decided in the window (ADR 0013).
type Digest struct {
	Summary   Summary
	Merges    []integration.MergedPR
	Decisions []Decision
	// Factory holds the pull requests that belong to a task of the factory, keyed "owner/name#N".
	Factory map[string]bool
	// MergeErr is set when a merge list could not be read in full; the digest then says so, next to
	// what was read, instead of showing an empty list, which would read as "nothing merged".
	MergeErr error
}

// gated is the label that marks a change to a protective path (AGENTS.md, "Ask before you do it").
const gated = "needs-owner-review"

// Limits of the message. They hold for every length of the window: the lists show their first
// lines and count the rest, so a busy night is as short to read as a quiet one.
const (
	maxMergeLines    = 10
	maxDecisionLines = 5
	maxDigestRunes   = 3000
)

// Format renders the digest as blocks separated by a rule: the head (window, counts, what the
// owner must do), "Needs you", "Merged" and "Decisions". Never longer than maxDigestRunes.
func (d Digest) Format() string {
	extra := d.mergeCounts()
	if d.MergeErr != nil {
		extra += "\nThe merge list is incomplete: " + short(d.MergeErr.Error(), 200)
	}
	return capText(join(d.Summary.head("Morning digest", extra), d.Summary.needsYouBlock(), d.mergedBlock(), d.decisionsBlock()))
}

type mergeGroup int

const (
	groupGated mergeGroup = iota
	groupUnreviewed
	groupOther
)

var groupTitle = [...]string{
	groupGated:      "Protected paths, look at these first",
	groupUnreviewed: "Code without a recorded independent review",
	groupOther:      "Everything else",
}

// group says how closely the owner should look at a merge: a protected path first, then code
// with no recorded independent review (the rule skips docs-only changes), then the rest.
func group(m integration.MergedPR) mergeGroup {
	switch {
	case slices.Contains(m.Labels, gated):
		return groupGated
	case !m.Reviewed && !isDocs(m.Title):
		return groupUnreviewed
	}
	return groupOther
}

func isDocs(title string) bool { return strings.HasPrefix(strings.ToLower(title), "docs") }

// repoOf is "owner/name" of a pull request URL, empty when the URL has another shape.
func repoOf(url string) string {
	base, _, ok := strings.Cut(url, "/pull/")
	if !ok {
		return ""
	}
	parts := strings.Split(base, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

func (d Digest) byFactory(m integration.MergedPR) bool {
	return d.Factory[fmt.Sprintf("%s#%d", repoOf(m.URL), m.Number)]
}

// mergeCounts is the line of the head that tells how many merges there were and how many need a look.
func (d Digest) mergeCounts() string {
	if d.MergeErr != nil && len(d.Merges) == 0 {
		return "Merged pull requests: unknown."
	}
	var n [3]int
	factory := 0
	for _, m := range d.Merges {
		n[group(m)]++
		if d.byFactory(m) {
			factory++
		}
	}
	if len(d.Merges) == 0 {
		return "No pull requests merged."
	}
	line := fmt.Sprintf("%d pull request%s merged (%d by the factory's own tasks)", len(d.Merges), plural(int64(len(d.Merges)), "s", ""), factory)
	if n[groupGated]+n[groupUnreviewed] > 0 {
		line += fmt.Sprintf(": %d on protected paths, %d code changes without a recorded review", n[groupGated], n[groupUnreviewed])
	}
	return line + "."
}

// mergedBlock lists the merges that need a look, up to maxMergeLines, then says how many are left
// and links to the full list on the code host. Each line is a pull request with its page, where
// the Revert button is.
func (d Digest) mergedBlock() string {
	if len(d.Merges) == 0 {
		return ""
	}
	multi := false
	for _, m := range d.Merges {
		multi = multi || repoOf(m.URL) != repoOf(d.Merges[0].URL)
	}
	lines := []string{fmt.Sprintf("*Merged* (%d). Revert: press Revert on the pull request page.", len(d.Merges))}
	shown := 0
	left := map[string]int{} // repository base URL -> merges not shown
	for g := groupGated; g <= groupOther; g++ {
		members := d.inGroup(g)
		if len(members) == 0 {
			continue
		}
		if shown >= maxMergeLines {
			// Nothing left in the budget: the rest are only counted, under no heading.
			for _, m := range members {
				left[base(m.URL)]++
			}
			continue
		}
		lines = append(lines, fmt.Sprintf("%s (%d):", groupTitle[g], len(members)))
		for _, m := range members {
			if shown == maxMergeLines {
				left[base(m.URL)]++
				continue
			}
			shown++
			lines = append(lines, mergeLine(m, multi))
		}
	}
	return strings.Join(append(lines, d.moreLines(left)...), "\n")
}

func (d Digest) inGroup(g mergeGroup) []integration.MergedPR {
	var out []integration.MergedPR
	for _, m := range d.Merges {
		if group(m) == g {
			out = append(out, m)
		}
	}
	return out
}

func mergeLine(m integration.MergedPR, multi bool) string {
	ref := fmt.Sprintf("#%d", m.Number)
	if multi {
		ref = repoOf(m.URL) + ref
	}
	return fmt.Sprintf("• %s %s %s", ref, short(m.Title, maxTitle), m.URL)
}

// moreLines counts the merges that are not listed, with a link to the full list per repository.
func (d Digest) moreLines(left map[string]int) []string {
	var out []string
	for _, b := range slices.Sorted(maps.Keys(left)) {
		more := fmt.Sprintf("• %d more", left[b])
		if b != "" {
			more += ": " + b + "/pulls?q=" + url.QueryEscape("is:pr is:merged merged:>="+d.Summary.Since.UTC().Format(time.RFC3339))
		}
		out = append(out, more)
	}
	return out
}

func base(u string) string {
	b, _, _ := strings.Cut(u, "/pull/")
	return b
}

// decisionPhrase says in plain words what a move of a task was.
var decisionPhrase = map[string]string{
	"approve": "approved", "reject": "rejected", "request_changes": "changes requested",
	"merge": "merged", "hand_off": "handed off", "team_merged": "merged by the team",
	"pr_closed": "pull request closed", "merged_outside": "merged on GitHub", "closed_outside": "closed on GitHub",
	"cancel": "canceled", "block_decided": "block decided", "budget_exhausted": "stopped, budget used up",
	"rollback": "rolled back",
}

func (d Digest) decisionsBlock() string {
	if len(d.Decisions) == 0 {
		return ""
	}
	lines := []string{fmt.Sprintf("*Decisions* (%d)", len(d.Decisions))}
	for i, x := range d.Decisions {
		if i == maxDecisionLines {
			lines = append(lines, fmt.Sprintf("• %d more", len(d.Decisions)-maxDecisionLines))
			break
		}
		what := decisionPhrase[x.Event]
		if what == "" {
			what = strings.ReplaceAll(x.Event, "_", " ")
		}
		line := fmt.Sprintf("• %s %s: %s", x.At.Format("02 Jan 15:04"), what, short(x.Title, maxTitle))
		if x.Actor != "" && x.Actor != "system" {
			line += " (by " + x.Actor + ")"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// capText is the hard cap on the length of the message: it cuts at a line break and says so.
func capText(s string) string {
	r := []rune(s)
	if len(r) <= maxDigestRunes {
		return s
	}
	const note = "\n…cut to fit; see the pull request list."
	cut := string(r[:maxDigestRunes-len([]rune(note))])
	if i := strings.LastIndex(cut, "\n"); i > 0 {
		cut = cut[:i]
	}
	return cut + note
}

// BuildDigest reads the summary and the decisions from the book and the merges from the code
// host for each repository (owner/name). A repository whose list cannot be read puts its error
// in MergeErr, and the digest still goes out with what it has.
func BuildDigest(ctx context.Context, pool *pgxpool.Pool, log integration.MergeLog, tenant string, repos []string, since, until time.Time, fallback ...string) (Digest, error) {
	s, err := Build(ctx, pool, tenant, since, until, fallback...)
	if err != nil {
		return Digest{}, err
	}
	dec, err := Decisions(ctx, pool, tenant, since)
	if err != nil {
		return Digest{}, err
	}
	d := Digest{Summary: s, Decisions: dec, Factory: map[string]bool{}}
	q := bookdb.New(pool)
	for _, st := range []flow.State{flow.Released, flow.MergedByTeam, flow.HandedOff} {
		tasks, err := q.ListTasksByState(ctx, bookdb.ListTasksByStateParams{Tenant: tenant, State: string(st)})
		if err != nil {
			return Digest{}, fmt.Errorf("summary: tasks by state: %w", err)
		}
		for _, t := range tasks {
			if t.PrNumber.Valid {
				d.Factory[fmt.Sprintf("%s#%d", repoNameOnly(t.Repo), t.PrNumber.Int32)] = true
			}
		}
	}
	for _, r := range repos {
		ms, err := log.MergedSince(ctx, r, since)
		if err != nil {
			d.MergeErr = errors.Join(d.MergeErr, fmt.Errorf("%s: %w", r, err))
		}
		d.Merges = append(d.Merges, ms...)
	}
	return d, nil
}

// repoNameOnly drops the host from a repository as tasks keep it (host/owner/name).
func repoNameOnly(repo string) string {
	if _, rest, ok := strings.Cut(repo, "/"); ok && strings.Contains(rest, "/") {
		return rest
	}
	return repo
}
