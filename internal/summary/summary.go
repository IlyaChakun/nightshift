// Package summary builds the morning summary and the `/ns status` answer from the book
// (design sections 8 and 15). It is plain SQL and text: no model writes it, so the numbers
// are always the book's.
package summary

import (
	"context"
	"fmt"
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
	Finished int64 // tasks that ended or were handed off in the window
	Active   int64
	Waiting  map[flow.State]int64
	// Merged, WithoutChanges and Median are over all merged tasks of the tenant.
	Merged         int64
	WithoutChanges int64
	Median         time.Duration
}

// Build reads the numbers for a tenant, for the window that starts at since.
func Build(ctx context.Context, pool *pgxpool.Pool, tenant string, since time.Time) (Summary, error) {
	q := bookdb.New(pool)
	counts, err := q.CountTasksByState(ctx, tenant)
	if err != nil {
		return Summary{}, fmt.Errorf("summary: %w", err)
	}
	s := Summary{Since: since, Waiting: map[flow.State]int64{}}
	for _, c := range counts {
		st := flow.State(c.State)
		switch {
		case slices.Contains(waitingStates, st):
			s.Waiting[st] = c.Tasks
		case slices.Contains(activeStates, st):
			s.Active += c.Tasks
		}
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
	return s, nil
}

// Format renders one short message.
func (s Summary) Format() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Since %s: %d finished, %d in work.\n", s.Since.UTC().Format("02 Jan 15:04 UTC"), s.Finished, s.Active)
	var wait []string
	total := int64(0)
	for _, st := range waitingStates {
		if n := s.Waiting[st]; n > 0 {
			wait = append(wait, fmt.Sprintf("%d %s", n, strings.ReplaceAll(string(st), "_", " ")))
			total += n
		}
	}
	if total == 0 {
		b.WriteString("Nothing waits for you.\n")
	} else {
		fmt.Fprintf(&b, "Waiting for you: %d (%s).\n", total, strings.Join(wait, ", "))
	}
	if s.Merged == 0 {
		b.WriteString("No merged tasks yet, so no merge numbers.")
	} else {
		fmt.Fprintf(&b, "Accepted without your changes: %d of %d (%d%%). Median time to merge: %s.",
			s.WithoutChanges, s.Merged, 100*s.WithoutChanges/s.Merged, duration(s.Median))
	}
	return b.String()
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
