package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// Plan says when a queued task may start and how it ranks against the others, so a task can
// be written during the day and run at night. The zero value starts at once at priority 0.
type Plan struct {
	NotBefore time.Time
	Priority  int // -10 to 10; a higher one goes first
	// AllowFallback lets the task run on a fallback provider when the primary ones are at a
	// long limit (tenant setting on_limit.fallback decides the default).
	AllowFallback bool
}

// ErrNewTask means the task cannot be queued as asked.
var ErrNewTask = errors.New("cannot add the task")

// AddTask queues a task the owner typed, with the event that says so, until Lead turns chat
// ideas into queued tasks. repo is owner/name; empty means the tenant's first repository.
func AddTask(ctx context.Context, pool *pgxpool.Pool, t config.Tenant, repo, text, actor string, plan Plan) (bookdb.BookTask, error) {
	var none bookdb.BookTask
	if plan.Priority < -10 || plan.Priority > 10 {
		return none, fmt.Errorf("%w: priority %d is outside -10 to 10", ErrNewTask, plan.Priority)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return none, fmt.Errorf("%w: the title is empty", ErrNewTask)
	}
	repos := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		repos = append(repos, repoName(r))
	}
	if repo == "" && len(repos) > 0 {
		repo = repos[0]
	}
	// Only a tenant's own repository: its credential and review rules apply to nothing else.
	if !slices.Contains(repos, repo) {
		return none, fmt.Errorf("%w: tenant %s has no repository %q (it has: %s)", ErrNewTask, t.Name, repo, strings.Join(repos, ", "))
	}
	masked := NewMasker().String(text)
	payload, err := json.Marshal(map[string]string{"text": masked})
	if err != nil {
		return none, fmt.Errorf("add task: %w", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return none, fmt.Errorf("add task: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := bookdb.New(tx)
	notBefore := pgtype.Timestamptz{Time: plan.NotBefore, Valid: !plan.NotBefore.IsZero()}
	task, err := q.CreateQueuedTask(ctx, bookdb.CreateQueuedTaskParams{
		Tenant: t.Name, Repo: repo, Title: title(masked), NotBefore: notBefore, Priority: int32(plan.Priority), AllowFallback: plan.AllowFallback,
	})
	if err != nil {
		return none, fmt.Errorf("add task: %w", err)
	}
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: t.Name, TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: "task_created", Actor: actor, Payload: payload,
	}); err != nil {
		return none, fmt.Errorf("add task: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return none, fmt.Errorf("add task: %w", err)
	}
	return task, nil
}
