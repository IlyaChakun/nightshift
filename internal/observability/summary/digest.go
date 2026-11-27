package summary

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
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
	Merges    []MergedPR
	Decisions []Decision
	// Factory holds the pull requests that belong to a task of the factory, keyed "owner/name#N".
	Factory map[string]bool
	// MergeErr is set when a merge list could not be read in full; the digest then says so, next to
	// what was read, instead of showing an empty list, which would read as "nothing merged".
	MergeErr error
	// Plan is how far the repositories' plans have come; empty when no plan could be read.
	Plan []PlanProgress
	// Health is the state of the repositories (alerts, idle pull requests, main); nil when no source was given.
	Health *Health
}

// gated is the label that marks a change to a protective path (AGENTS.md, "Ask before you do it").
const gated = "needs-owner-review"

// Limits of the message. They hold for every length of the window: the lists show their first
// lines and count the rest, so a busy night is as short to read as a quiet one.
const (
	maxMergeLines    = 8
	maxDecisionLines = 5
	maxDigestRunes   = 3000
)

// Format renders the digest as blocks separated by a rule: the head (the window, what the owner
// must do, the state of main, the counts), "Needs you", "What changed", "Plan", "Merged",
// "Decisions", the health detail when something is wrong, and the conclusion. Never longer than
// maxDigestRunes.
func (d Digest) Format() string {
	// The head, "Needs you", "Merged" and the conclusion (when there is one) always show: the head
	// and the conclusion point at the two lists. "Merged" gives up lines, down to minMergeLines,
	// only when that lets more of the health detail, the counts of what changed and the plan show:
	// each of the three says more than one more pull request line. Fewer lines that place nothing
	// more are not taken, and fewer lines can be a longer block, since links replace them.
	conclusion := d.conclusion(true)
	room := maxDigestRunes - runes(sep) - runes(conclusion)
	optional := d.optional()
	var blocks [blockHealth + 1]string
	best := math.MinInt
	for budget := maxMergeLines; budget >= minMergeLines; budget-- {
		try := [blockHealth + 1]string{blockHead: d.head(), blockNeeds: d.Summary.needsYouBlock(), blockMerged: d.mergedBlock(budget)}
		// Too long without any optional block: the text will be cut, so the shortest loses least.
		size := runes(join(try[:]...))
		score, all := -size, 0
		if size <= room {
			fit(try[:], room, optional)
			score, all = essentialsIn(try[:], optional[:essentialBlocks])
		}
		if score > best {
			blocks, best = try, score
		}
		if size <= room && score == all {
			break
		}
	}
	body := join(blocks[:]...)
	if runes(body) > room {
		// Still too long (very long names): the text is cut at a line, so the conclusion must not
		// point at a list that may be gone.
		conclusion = d.conclusion(false)
		body = capText(body, maxDigestRunes-runes(sep)-runes(conclusion))
	}
	return join(body, conclusion)
}

// optionalBlock is a block that shows only when it fits, and where it goes in the message.
type optionalBlock struct {
	at   int
	text string
}

// essentialBlocks is how many of the optional blocks, from the first, "Merged" gives up lines for.
const essentialBlocks = 3

// essentialsIn scores which of the essential blocks are in the message, an earlier one above all
// later ones together, and gives the score of a message that has every one with text.
func essentialsIn(blocks []string, essential []optionalBlock) (got, all int) {
	for i, b := range essential {
		if b.text == "" {
			continue
		}
		weight := 1 << (len(essential) - 1 - i)
		all += weight
		if blocks[b.at] != "" {
			got += weight
		}
	}
	return got, all
}

// optional lists the blocks that show only when they fit, the most useful first: the health
// detail (there only when something is wrong), what changed as one line of counts, the plan, then
// the titles of what changed in place of the counts, then the decisions.
func (d Digest) optional() []optionalBlock {
	var health string
	if len(d.Health.problems()) > 0 {
		health = d.Health.block()
	}
	return []optionalBlock{
		{blockHealth, health},
		{blockChanged, d.changedCounts()},
		{blockPlan, planBlock(d.Plan)},
		{blockChanged, d.changedBlock()},
		{blockDecisions, d.decisionsBlock()},
	}
}

// fit puts each block of add into blocks, in order, unless the joined message would then be
// longer than room; a block that does not fit leaves what was in its place. So a small block is
// never lost because a larger one before it did not fit.
func fit(blocks []string, room int, add []optionalBlock) {
	for _, b := range add {
		before := blocks[b.at]
		blocks[b.at] = b.text
		if runes(join(blocks...)) > room {
			blocks[b.at] = before
		}
	}
}

// The blocks of the message, in the order they are shown.
const (
	blockHead = iota
	blockNeeds
	blockChanged
	blockPlan
	blockMerged
	blockDecisions
	blockHealth
)

func runes(s string) int { return len([]rune(s)) }

// head is the block that comes first and answers at a glance: does anything wait for the owner,
// is the factory healthy, how much was merged and how the factory's own tasks went.
func (d Digest) head() string {
	s := d.Summary
	var b strings.Builder
	fmt.Fprintf(&b, "*Morning digest* · %s\n", s.window())
	if n := s.waitingTotal(); n == 0 {
		b.WriteString("Nothing waits for you.")
	} else {
		fmt.Fprintf(&b, "%d need%s you.", n, plural(n, "", "s"))
	}
	if v := d.Health.verdict(); v != "" {
		b.WriteString(" " + v)
	}
	b.WriteString("\n" + d.mergeCounts())
	if d.MergeErr != nil {
		b.WriteString("\nThe merge list is incomplete: " + short(d.MergeErr.Error(), 200))
	}
	fmt.Fprintf(&b, "\nThe factory's own tasks: %d finished, %d in work.", s.Finished, s.Active)
	if s.Merged > 0 {
		// These two cover every task, not the window; the line says so.
		fmt.Fprintf(&b, " All time: %d of %d accepted without your changes, median time to merge %s.",
			s.WithoutChanges, s.Merged, duration(s.Median))
	}
	if line := s.limitLine(); line != "" {
		b.WriteString("\n" + line)
	}
	return b.String()
}

type mergeGroup int

const (
	groupFactory mergeGroup = iota
	groupGated
	groupUnreviewed
	groupOther
)

// Lines of "Merged". When every group has more than fits, the factory's own work and the
// protected paths (which also get a link to their full list) each keep two, and code without a
// recorded review, which no link can list, takes the rest. minMergeLines is what the block keeps
// when the message is too long: one line for each group the conclusion counts.
const (
	maxFactoryLines = 2
	minGatedLines   = 2
	minMergeLines   = 2
	maxRepoLinks    = 2
)

var groupTitle = [...]string{
	groupFactory:    "The factory's own work",
	groupGated:      "Protected paths",
	groupUnreviewed: "Code without a recorded independent review",
	groupOther:      "Reviewed code and docs",
}

// listOrder is the order of the groups in "Merged": the protected paths first (ADR 0013: the label
// is there so they can be read first), then code that went in against the review rule, then the
// factory's own work, then everything else, which is listed only on a day with lines to spare.
var listOrder = [...]mergeGroup{groupGated, groupUnreviewed, groupFactory, groupOther}

// group says where a merge belongs: the factory's own work (a pull request of a task) first, then
// by how closely the owner should look: a protected path, then code with no recorded independent
// review (the rule skips docs-only changes), then the rest.
func (d Digest) group(m MergedPR) mergeGroup {
	switch {
	case d.byFactory(m):
		return groupFactory
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

func (d Digest) byFactory(m MergedPR) bool {
	return d.Factory[fmt.Sprintf("%s#%d", repoOf(m.URL), m.Number)]
}

// mergeCounts is the line of the head that says how much was merged and by whom. What needs a
// look is counted in one place, the conclusion.
func (d Digest) mergeCounts() string {
	if d.MergeErr != nil && len(d.Merges) == 0 {
		return "Merged pull requests: unknown."
	}
	if len(d.Merges) == 0 {
		return "No pull requests merged."
	}
	own := 0
	for _, m := range d.Merges {
		if d.byFactory(m) {
			own++
		}
	}
	total := int64(len(d.Merges))
	return fmt.Sprintf("%d pull request%s merged: %d by the factory's own tasks, %d by sessions and people.",
		total, plural(total, "s", ""), own, len(d.Merges)-own)
}

// mergedBlock lists the merges that need a look, on at most budget lines, then says how many are
// left and links to the full list on the code host. Each line is a pull request with its page,
// where the Revert button is. Every group that is counted in the conclusion has lines here.
func (d Digest) mergedBlock(budget int) string {
	if len(d.Merges) == 0 {
		return ""
	}
	lines := []string{fmt.Sprintf("*Merged* (%d). Revert: press Revert on the pull request page.", len(d.Merges))}
	var members [groupOther + 1][]MergedPR
	var sizes [groupOther + 1]int
	for g := range members {
		members[g] = d.inGroup(mergeGroup(g))
		sizes[g] = len(members[g])
	}
	rooms := mergeRooms(budget, sizes)
	left := map[string]int{} // repository base URL -> merges not shown
	for _, g := range listOrder {
		ms, room := members[g], rooms[g]
		if room > 0 {
			lines = append(lines, fmt.Sprintf("%s (%d):", groupTitle[g], len(ms)))
		}
		for i, m := range ms {
			if i >= room {
				left[base(m.URL)]++
				continue
			}
			lines = append(lines, mergeLine(m))
		}
		if g == groupGated && len(ms) > room {
			lines = append(lines, d.gatedLinks(ms[room:])...)
		}
	}
	return strings.Join(append(lines, d.moreLines(left)...), "\n")
}

// mergeRooms shares budget lines of "Merged" between the groups, given their sizes. Each group the
// conclusion counts gets a line first; then the factory's own work and the protected paths get
// their few, code without a recorded review gets what is left, and lines nobody needs pass on,
// last to the merges that need no look.
func mergeRooms(budget int, sizes [groupOther + 1]int) [groupOther + 1]int {
	var r [groupOther + 1]int
	spare := budget
	give := func(g mergeGroup, upTo int) {
		add := max(0, min(spare, min(sizes[g], upTo)-r[g]))
		r[g] += add
		spare -= add
	}
	give(groupUnreviewed, 1)
	give(groupGated, 1)
	give(groupFactory, maxFactoryLines)
	give(groupGated, minGatedLines)
	for _, g := range [...]mergeGroup{groupUnreviewed, groupGated, groupFactory, groupOther} {
		give(g, budget)
	}
	return r
}

// gatedLinks links to the protected-path merges of each repository that has some unlisted: the
// label makes that list one search on the code host.
func (d Digest) gatedLinks(unlisted []MergedPR) []string {
	repos := map[string]int{}
	for _, m := range unlisted {
		if b := base(m.URL); b != "" {
			repos[b]++
		}
	}
	var out []string
	for i, b := range mostFirst(repos) {
		if i == maxRepoLinks { // a link is long; a tenant with many repositories gets the busiest ones
			break
		}
		out = append(out, "• all on protected paths: "+b+"/pulls?q="+url.QueryEscape("is:pr is:merged label:"+gated+" merged:>="+d.Summary.Since.UTC().Format(time.RFC3339)))
	}
	return out
}

// mostFirst lists the keys of n, the largest count first and equal counts by name.
func mostFirst(n map[string]int) []string {
	return slices.SortedFunc(maps.Keys(n), func(a, b string) int { return cmp.Or(cmp.Compare(n[b], n[a]), strings.Compare(a, b)) })
}

func (d Digest) inGroup(g mergeGroup) []MergedPR {
	var out []MergedPR
	for _, m := range d.Merges {
		if d.group(m) == g {
			out = append(out, m)
		}
	}
	// Newest first: the owner asks what happened last.
	slices.SortStableFunc(out, func(a, b MergedPR) int { return b.MergedAt.Compare(a.MergedAt) })
	return out
}

// mergeLine is one pull request: what it did and its page. The link carries the repository and
// the number, so they are not written twice; a merge without a link keeps its number.
func mergeLine(m MergedPR) string {
	if m.URL == "" {
		return fmt.Sprintf("• %s (#%d)", what(m), m.Number)
	}
	return fmt.Sprintf("• %s %s", what(m), m.URL)
}

// moreLines counts the merges that are not listed, with a link to the full list per repository;
// past maxRepoLinks repositories the rest is one count.
func (d Digest) moreLines(left map[string]int) []string {
	var out []string
	rest, restRepos := 0, 0
	for i, b := range mostFirst(left) {
		if i >= maxRepoLinks {
			rest += left[b]
			restRepos++
			continue
		}
		more := fmt.Sprintf("• %d more", left[b])
		if b != "" {
			more += ": " + b + "/pulls?q=" + url.QueryEscape("is:pr is:merged merged:>="+d.Summary.Since.UTC().Format(time.RFC3339))
		}
		out = append(out, more)
	}
	if rest > 0 {
		out = append(out, fmt.Sprintf("• %d more in %d other repositor%s", rest, restRepos, plural(int64(restRepos), "ies", "y")))
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
		line := fmt.Sprintf("• %s %s: %s", x.At.In(d.Summary.Since.Location()).Format("02 Jan 15:04"), what, short(x.Title, maxTitle))
		if x.Actor != "" && x.Actor != "system" {
			line += " (by " + x.Actor + ")"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// capText is the hard cap on the length of the message: it cuts at a line break and says so.
func capText(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	const note = "\n…cut to fit; see the pull request list."
	cut := string(r[:limit-len([]rune(note))])
	if i := strings.LastIndex(cut, "\n"); i > 0 {
		cut = cut[:i]
	}
	return cut + note
}

// BuildDigest reads the summary and the decisions from the book and the merges from the code
// host for each repository (owner/name). A repository whose list cannot be read puts its error
// in MergeErr, and the digest still goes out with what it has.
func BuildDigest(ctx context.Context, pool *pgxpool.Pool, log MergeLog, tenant string, repos []string, since, until time.Time, fallback ...string) (Digest, error) {
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
