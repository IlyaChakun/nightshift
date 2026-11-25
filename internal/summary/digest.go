package summary

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
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
	// MergeErr is set when a merge list could not be read in full; the digest then says so, next to
	// what was read, instead of showing an empty list, which would read as "nothing merged".
	MergeErr error
}

// gated is the label that marks a change to a protective path (AGENTS.md, "Ask before you do it").
const gated = "needs-owner-review"

// Format renders the digest: the summary, then each merge with its reason and a revert link,
// gated and unreviewed ones first, then the decisions.
func (d Digest) Format() string {
	var b strings.Builder
	b.WriteString(d.Summary.Format())
	b.WriteString("\n\n")
	switch {
	case len(d.Merges) == 0 && d.MergeErr == nil:
		b.WriteString("Merged: nothing.\n")
	case len(d.Merges) == 0:
		b.WriteString("Merged: unknown.\n")
	default:
		fmt.Fprintf(&b, "Merged: %d.\n", len(d.Merges))
		for _, m := range orderMerges(d.Merges) {
			b.WriteString(mergeLine(m))
		}
	}
	if d.MergeErr != nil {
		fmt.Fprintf(&b, "The merge list is incomplete: %v\n", d.MergeErr)
	}
	if len(d.Decisions) > 0 {
		fmt.Fprintf(&b, "Decisions: %d.\n", len(d.Decisions))
		for _, x := range d.Decisions {
			fmt.Fprintf(&b, "- %s #%d %q: %s -> %s, by %s\n", x.At.UTC().Format("02 Jan 15:04"), x.TaskID, x.Title, x.From, x.To, x.Actor)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// orderMerges puts what needs a look first: gated, then unreviewed, then the rest, each in time order.
func orderMerges(in []integration.MergedPR) []integration.MergedPR {
	rank := func(m integration.MergedPR) int {
		switch {
		case hasLabel(m, gated):
			return 0
		case !m.Reviewed:
			return 1
		}
		return 2
	}
	out := make([]integration.MergedPR, 0, len(in))
	for r := range 3 {
		for _, m := range in {
			if rank(m) == r {
				out = append(out, m)
			}
		}
	}
	return out
}

func hasLabel(m integration.MergedPR, l string) bool {
	return slices.Contains(m.Labels, l)
}

func mergeLine(m integration.MergedPR) string {
	var flags []string
	if hasLabel(m, gated) {
		flags = append(flags, "gated")
	}
	if !m.Reviewed {
		flags = append(flags, "no independent review recorded")
	}
	line := fmt.Sprintf("- #%d %s", m.Number, m.Title)
	if m.MergedBy != "" {
		line += " (by " + m.MergedBy + ")"
	}
	if len(flags) > 0 {
		line += " [" + strings.Join(flags, ", ") + "]"
	}
	line += "\n"
	if m.Reason != "" {
		line += "  why: " + m.Reason + "\n"
	}
	return line + "  revert: " + m.URL + " (press Revert on the page)\n"
}

// BuildDigest reads the summary and the decisions from the book and the merges from the code
// host for each repository (owner/name). A repository whose list cannot be read puts its error
// in MergeErr, and the digest still goes out with what it has.
func BuildDigest(ctx context.Context, pool *pgxpool.Pool, log integration.MergeLog, tenant string, repos []string, since time.Time, fallback ...string) (Digest, error) {
	s, err := Build(ctx, pool, tenant, since, fallback...)
	if err != nil {
		return Digest{}, err
	}
	dec, err := Decisions(ctx, pool, tenant, since)
	if err != nil {
		return Digest{}, err
	}
	d := Digest{Summary: s, Decisions: dec}
	for _, r := range repos {
		ms, err := log.MergedSince(ctx, r, since)
		if err != nil {
			d.MergeErr = errors.Join(d.MergeErr, fmt.Errorf("%s: %w", r, err))
		}
		d.Merges = append(d.Merges, ms...)
	}
	return d, nil
}
