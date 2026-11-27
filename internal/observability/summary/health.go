package summary

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// IdlePR is how long an open pull request may go without an update before the digest lists it.
const IdlePR = 24 * time.Hour

// listedAlerts is how many alerts the digest names one by one; the rest are counted.
const listedAlerts = 5

// Health is what the morning digest says about the repositories themselves: open alerts, idle
// pull requests and a red default branch. A part that could not be read is an error, never an
// empty list, so "unknown" does not read as "fine".
type Health struct {
	Alerts     []integration.Alert
	AlertsErr  error
	Idle       []integration.PRActivity
	PRsErr     error
	MainFailed []string
	MainErr    error
	Now        time.Time
}

// BuildHealth reads the three signals for each repository (owner/name). Without a source the
// digest has no health part.
func BuildHealth(ctx context.Context, src integration.RepoHealth, repos []string, now time.Time) *Health {
	if src == nil {
		return nil
	}
	h := &Health{Now: now}
	for _, r := range repos {
		a, err := src.OpenAlerts(ctx, r)
		h.AlertsErr = errors.Join(h.AlertsErr, wrap(r, err))
		for i := range a {
			a[i].Repo = r
		}
		h.Alerts = append(h.Alerts, a...)
		prs, err := src.OpenPullRequests(ctx, r)
		h.PRsErr = errors.Join(h.PRsErr, wrap(r, err))
		for _, p := range prs {
			p.Repo = r
			if now.Sub(p.Updated) >= IdlePR {
				h.Idle = append(h.Idle, p)
			}
		}
		f, err := src.DefaultBranchFailures(ctx, r)
		h.MainErr = errors.Join(h.MainErr, wrap(r, err))
		for _, name := range f {
			h.MainFailed = append(h.MainFailed, r+": "+name)
		}
	}
	return h
}

func wrap(repo string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", repo, err)
}

// block renders the health part for the digest; empty without a source.
func (h *Health) block() string {
	if h == nil {
		return ""
	}
	return h.Format()
}

// Format renders the health part: one line per signal, worst first.
func (h *Health) Format() string {
	var b strings.Builder
	b.WriteString("*Factory health*\n")
	switch {
	case h.MainErr != nil:
		fmt.Fprintf(&b, "• main: unknown (%v)\n", h.MainErr)
	case len(h.MainFailed) > 0:
		fmt.Fprintf(&b, "• main is RED: %s\n", strings.Join(h.MainFailed, ", "))
	default:
		b.WriteString("• main: green\n")
	}
	h.formatAlerts(&b)
	switch {
	case h.PRsErr != nil:
		fmt.Fprintf(&b, "• open pull requests: unknown (%v)\n", h.PRsErr)
	case len(h.Idle) == 0:
		b.WriteString("• pull requests: none idle for 24 hours\n")
	default:
		fmt.Fprintf(&b, "• %d pull request(s) idle for 24 hours or more:\n", len(h.Idle))
		for _, p := range h.Idle[:min(len(h.Idle), listedAlerts)] {
			draft := ""
			if p.Draft {
				draft = " (draft)"
			}
			fmt.Fprintf(&b, "   #%d %s%s, idle %s\n", p.Number, p.Title, draft, duration(h.Now.Sub(p.Updated)))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (h *Health) formatAlerts(b *strings.Builder) {
	if h.AlertsErr != nil {
		fmt.Fprintf(b, "• code-scanning alerts: unknown (%v)\n", h.AlertsErr)
		return
	}
	if len(h.Alerts) == 0 {
		b.WriteString("• code-scanning alerts: none open\n")
		return
	}
	alerts := slices.Clone(h.Alerts)
	rank := map[string]int{"critical": 0, "high": 1, "error": 1, "medium": 2, "warning": 2}
	slices.SortStableFunc(alerts, func(x, y integration.Alert) int {
		rx, ry := rank[x.Severity], rank[y.Severity]
		if rx == 0 && x.Severity != "critical" {
			rx = 3
		}
		if ry == 0 && y.Severity != "critical" {
			ry = 3
		}
		if rx != ry {
			return rx - ry
		}
		return x.Created.Compare(y.Created) // oldest first
	})
	fmt.Fprintf(b, "• %d code-scanning alert(s) open, oldest %s:\n", len(alerts), duration(h.Now.Sub(slices.MinFunc(alerts, func(x, y integration.Alert) int { return x.Created.Compare(y.Created) }).Created)))
	for _, a := range alerts[:min(len(alerts), listedAlerts)] {
		fmt.Fprintf(b, "   #%d %s %s in %s, open %s\n", a.Number, a.Severity, a.Rule, a.Path, duration(h.Now.Sub(a.Created)))
	}
	if len(alerts) > listedAlerts {
		fmt.Fprintf(b, "   and %d more\n", len(alerts)-listedAlerts)
	}
}
