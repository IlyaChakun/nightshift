package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
)

// Decision says what to do with a job that needs a provider.
type Decision int

// Decisions of the router.
const (
	// Run: enqueue the job on Choice.Provider.
	Run Decision = iota
	// Wait: every provider in reach is at its concurrency cap; try again soon.
	Wait
	// Park: every provider hit a limit; the task waits until Choice.Until and the owner
	// gets a blocker in chat (design section 12).
	Park
)

// Choice is the router's answer.
type Choice struct {
	Decision Decision
	Provider string
	Until    time.Time // for Park: when the earliest limit resets
}

// Router picks a provider for a role from the tenant's ordered list. A provider that hit
// a plan or rate limit is skipped until its reset; one at its concurrency cap is skipped
// for now. A limit is read from the book (see limit), so a restart does not forget it.
type Router struct {
	Providers map[string]config.Provider
	Chains    map[string][]string // role -> providers in order of preference
	// InFlight counts the queued and running jobs of a provider; nil reads the book.
	InFlight func(ctx context.Context, provider string) (int, error)
	Pool     *pgxpool.Pool
	Now      func() time.Time
	// MaxWait is the longest limit reset worth waiting for before a fallback provider is used;
	// zero means 2 hours.
	MaxWait time.Duration

	mu      sync.Mutex
	limited map[string]time.Time
}

// ErrNoProvider means the tenant lists no provider for the role.
var ErrNoProvider = errors.New("dispatch: no provider for role")

func (r *Router) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// NoteLimit records that a provider is out of quota until the given time.
func (r *Router) NoteLimit(provider string, until time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.limited == nil {
		r.limited = map[string]time.Time{}
	}
	r.limited[provider] = until
}

// Choose picks a provider for a role that may not use a fallback provider.
func (r *Router) Choose(ctx context.Context, role string) (Choice, error) {
	return r.ChooseFor(ctx, role, false)
}

// ChooseFor picks the first usable primary provider of the role's list. When every primary
// one is at a limit that resets later than MaxWait and fallbackOK is set, it takes the first
// usable fallback provider instead; a nearer reset is waited for. Lead never uses a fallback
// provider: a weaker model must not decide the spec (ADR 0009).
func (r *Router) ChooseFor(ctx context.Context, role string, fallbackOK bool) (Choice, error) {
	chain := r.Chains[role]
	if len(chain) == 0 {
		return Choice{}, fmt.Errorf("%w %q", ErrNoProvider, role)
	}
	var primary, fallback []string
	for _, name := range chain {
		p, ok := r.Providers[name]
		if !ok {
			return Choice{}, fmt.Errorf("dispatch: unknown provider %q", name)
		}
		switch {
		case p.Tier != "fallback":
			primary = append(primary, name)
		case fallbackOK && role != "lead":
			fallback = append(fallback, name)
		}
	}
	if len(primary) == 0 && len(fallback) == 0 {
		// Only fallback providers and no permission to use them: say so instead of waiting for ever.
		return Choice{}, fmt.Errorf("%w %q: its chain holds only fallback providers the task may not use", ErrNoProvider, role)
	}
	now := r.now()
	c, err := r.pick(ctx, primary, now)
	if err != nil || c.Decision != Park || len(fallback) == 0 {
		return c, err
	}
	if len(primary) > 0 && c.Until.Sub(now) <= r.maxWait() {
		return c, nil // the reset is near: wait for it
	}
	f, err := r.pick(ctx, fallback, now)
	if err != nil || f.Decision == Run {
		return f, err
	}
	if len(primary) == 0 || (!f.Until.IsZero() && f.Until.Before(c.Until)) {
		c.Until = f.Until
	}
	return c, nil
}

func (r *Router) maxWait() time.Duration {
	if r.MaxWait > 0 {
		return r.MaxWait
	}
	return 2 * time.Hour
}

// pick returns Run for the first usable provider of the list, Wait when one is only at its
// concurrency cap, and Park with the earliest reset when every one is at a limit.
func (r *Router) pick(ctx context.Context, names []string, now time.Time) (Choice, error) {
	var busy bool
	var earliest time.Time
	for _, name := range names {
		until, limited, err := r.limit(ctx, name, now)
		if err != nil {
			return Choice{}, err
		}
		if limited {
			if earliest.IsZero() || until.Before(earliest) {
				earliest = until
			}
			continue
		}
		full, err := r.atCap(ctx, name, r.Providers[name])
		if err != nil {
			return Choice{}, err
		}
		if full {
			busy = true
			continue
		}
		return Choice{Decision: Run, Provider: name}, nil
	}
	if busy {
		return Choice{Decision: Wait}, nil
	}
	return Choice{Decision: Park, Until: earliest}, nil
}

func (r *Router) limit(ctx context.Context, provider string, now time.Time) (time.Time, bool, error) {
	r.mu.Lock()
	until, ok := r.limited[provider]
	if ok && !until.After(now) {
		delete(r.limited, provider)
		ok = false
	}
	r.mu.Unlock()
	if ok {
		return until, true, nil
	}
	if r.Pool == nil {
		return time.Time{}, false, nil
	}
	until, err := r.bookLimit(ctx, provider)
	if err != nil || !until.After(now) {
		return time.Time{}, false, err
	}
	return until, true, nil
}

// LimitBackoff is the wait after the nth limit in a row on one provider when the provider
// gave no reset time: 15 minutes, doubled each time, capped at 6 hours.
func LimitBackoff(n int) time.Duration {
	d := 15 * time.Minute
	for i := 1; i < n && d < 6*time.Hour; i++ {
		d *= 2
	}
	return min(d, 6*time.Hour)
}

// bookLimit reads the reset time of a provider from its latest job that hit a limit, or
// the zero time when the provider has worked since. Without a reset time from the provider
// the wait grows with the limits in a row.
func (r *Router) bookLimit(ctx context.Context, provider string) (time.Time, error) {
	var finished pgtype.Timestamptz
	var usage []byte
	var n int
	err := r.Pool.QueryRow(ctx, `
SELECT j.finished_at, j.usage,
       (SELECT count(*) FROM book.jobs x WHERE x.provider = $1 AND x.exit_reason = 'rate_limit'
          AND x.id > coalesce((SELECT max(id) FROM book.jobs WHERE provider = $1 AND state = 'done'), 0))
FROM book.jobs j
WHERE j.provider = $1 AND j.exit_reason = 'rate_limit' AND j.finished_at IS NOT NULL
ORDER BY j.id DESC LIMIT 1`, provider).Scan(&finished, &usage, &n)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("dispatch: limit of %s: %w", provider, err)
	}
	var u jobsapi.Usage
	if len(usage) > 0 {
		if err := json.Unmarshal(usage, &u); err != nil {
			return time.Time{}, fmt.Errorf("dispatch: usage of a %s job: %w", provider, err)
		}
	}
	if !u.LimitUntil.IsZero() {
		return u.LimitUntil, nil
	}
	return finished.Time.Add(LimitBackoff(n)), nil
}

// atCap reports whether a provider already runs as many jobs as its concurrency allows.
func (r *Router) atCap(ctx context.Context, name string, p config.Provider) (bool, error) {
	if p.Concurrency <= 0 {
		return false, nil
	}
	n, err := r.inFlight(ctx, name)
	return n >= p.Concurrency, err
}

func (r *Router) inFlight(ctx context.Context, provider string) (int, error) {
	if r.InFlight != nil {
		return r.InFlight(ctx, provider)
	}
	var n int
	err := r.Pool.QueryRow(ctx, `SELECT count(*) FROM book.jobs WHERE provider = $1 AND state IN ('queued', 'running')`, provider).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("dispatch: count jobs of %s: %w", provider, err)
	}
	return n, nil
}

// EnqueueJob routes a role job and stores it. Unless the choice is Run, nothing is stored.
// The harness session of (task, role) is continued only on the provider that created it;
// on another provider the context is rebuilt from the book (design section 12).
func (r *Router) EnqueueJob(ctx context.Context, q *bookdb.Queries, tenant string, taskID int64, role string, job jobsapi.Job) (Choice, error) {
	c, err := r.Choose(ctx, role)
	if err != nil || c.Decision != Run {
		return c, err
	}
	job.Provider, job.Role, job.Tenant = c.Provider, role, tenant
	job.Traceparent = Traceparent(ctx)
	pv := r.Providers[c.Provider]
	job.Harness, job.Auth, job.Model, job.BaseURL, job.KeyEnv = pv.Harness, pv.Auth, pv.Model, pv.BaseURL, pv.KeyEnv
	job.SessionID = ""
	sess, err := q.GetHarnessSession(ctx, bookdb.GetHarnessSessionParams{TaskID: taskID, Role: role})
	switch {
	case err == nil && sess.Provider == c.Provider:
		job.SessionID = sess.SessionID
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return Choice{}, fmt.Errorf("dispatch: session of task %d: %w", taskID, err)
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return Choice{}, fmt.Errorf("dispatch: encode job: %w", err)
	}
	if _, err := q.EnqueueJob(ctx, bookdb.EnqueueJobParams{
		Tenant: tenant, TaskID: taskID, Role: role, Provider: c.Provider, Payload: payload,
	}); err != nil {
		return Choice{}, fmt.Errorf("dispatch: enqueue job: %w", err)
	}
	return c, nil
}

// Traceparent renders the span in ctx as a W3C traceparent header value, or "" without one.
func Traceparent(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return fmt.Sprintf("00-%s-%s-%s", sc.TraceID(), sc.SpanID(), sc.TraceFlags())
}

// Spend charges a finished job to the task budget and reports whether the budget is used
// up. A zero maximum means no limit. The caller turns true into flow.BudgetExhausted.
func Spend(ctx context.Context, q *bookdb.Queries, taskID int64, iterations int32, tokens int64) (bool, error) {
	b, err := q.SpendBudget(ctx, bookdb.SpendBudgetParams{TaskID: taskID, Iterations: iterations, Tokens: tokens})
	if err != nil {
		return false, fmt.Errorf("dispatch: spend budget of task %d: %w", taskID, err)
	}
	return flow.Exhausted(b), nil
}

// limitHit reports whether a failed job stopped on a provider limit.
func limitHit(job bookdb.BookJob) bool {
	return job.ExitReason.String == "rate_limit"
}

// requeueAfterLimit closes a job that hit a limit without touching the task. The book now
// holds the limit, so the next tick routes to another provider or waits for the reset.
func (c *Conductor) requeueAfterLimit(ctx context.Context, task bookdb.BookTask, job bookdb.BookJob) error {
	if _, err := bookdb.New(c.Pool).ConsumeJob(ctx, job.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("conductor: %w", err)
	}
	c.log().InfoContext(ctx, "provider limit, job returns to the queue", "task", task.ID, "provider", job.Provider, "role", job.Role)
	return nil
}
