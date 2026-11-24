package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
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
// for now. Limits live in memory: after a restart the next run finds them again.
type Router struct {
	Providers map[string]config.Provider
	Chains    map[string][]string // role -> providers in order of preference
	// InFlight counts the queued and running jobs of a provider; nil reads the book.
	InFlight func(ctx context.Context, provider string) (int, error)
	Pool     *pgxpool.Pool
	Now      func() time.Time

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

// Choose picks the first usable provider of the role's list.
func (r *Router) Choose(ctx context.Context, role string) (Choice, error) {
	chain := r.Chains[role]
	if len(chain) == 0 {
		return Choice{}, fmt.Errorf("%w %q", ErrNoProvider, role)
	}
	now := r.now()
	var busy bool
	var earliest time.Time
	for _, name := range chain {
		p, ok := r.Providers[name]
		if !ok {
			return Choice{}, fmt.Errorf("dispatch: unknown provider %q", name)
		}
		if until, limited := r.limit(name, now); limited {
			if earliest.IsZero() || until.Before(earliest) {
				earliest = until
			}
			continue
		}
		if p.Concurrency > 0 {
			n, err := r.inFlight(ctx, name)
			if err != nil {
				return Choice{}, err
			}
			if n >= p.Concurrency {
				busy = true
				continue
			}
		}
		return Choice{Decision: Run, Provider: name}, nil
	}
	if busy {
		return Choice{Decision: Wait}, nil
	}
	return Choice{Decision: Park, Until: earliest}, nil
}

func (r *Router) limit(provider string, now time.Time) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	until, ok := r.limited[provider]
	if ok && !until.After(now) {
		delete(r.limited, provider)
		return time.Time{}, false
	}
	return until, ok
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
	job.Harness, job.Auth = r.Providers[c.Provider].Harness, r.Providers[c.Provider].Auth
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
	return exhausted(b), nil
}

func exhausted(b bookdb.BookBudget) bool {
	return (b.MaxIterations > 0 && b.UsedIterations >= b.MaxIterations) ||
		(b.MaxTokens > 0 && b.UsedTokens >= b.MaxTokens)
}
