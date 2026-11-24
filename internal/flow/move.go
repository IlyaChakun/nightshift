package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

// ErrConflict means the task changed between the read and the write: another decision won
// the compare-and-set. The caller reloads the task and decides again.
var ErrConflict = errors.New("flow: the task changed since it was read")

// Mover applies events to tasks in the book.
type Mover struct {
	Pool *pgxpool.Pool
}

// Move is one event for one task.
type Move struct {
	TaskID  int64
	Event   Event
	Actor   string // 'system', a role, or the actor's chat, GitHub or tracker id
	Profile Profile
	// NeedsHumanReview is the review policy's verdict (design section 11).
	NeedsHumanReview bool
	// HeadSHA is the new head of the task's PR; empty keeps the current one.
	HeadSHA string
	TraceID string
}

// Apply reads the task, asks the state machine for the next state and writes it with the
// compare-and-set of design section 7, all in one transaction together with the event that
// records it. A lost race returns ErrConflict and changes nothing; an event the state does
// not allow returns ErrNotAllowed.
func (m *Mover) Apply(ctx context.Context, mv Move) (bookdb.BookTask, error) {
	var moved bookdb.BookTask
	err := pgx.BeginFunc(ctx, m.Pool, func(tx pgx.Tx) error {
		q := bookdb.New(tx)
		cur, err := q.GetTask(ctx, mv.TaskID)
		if err != nil {
			return fmt.Errorf("flow: task %d: %w", mv.TaskID, err)
		}
		spent, err := budgetSpent(ctx, q, cur.ID)
		if err != nil {
			return err
		}
		out, err := Apply(Task{
			State: State(cur.State), Profile: mv.Profile, Returns: int(cur.Returns),
			NeedsHumanReview: mv.NeedsHumanReview, BudgetExhausted: spent,
		}, mv.Event)
		if err != nil {
			return err
		}
		head := cur.HeadSha
		if mv.HeadSHA != "" {
			head = pgtype.Text{String: mv.HeadSHA, Valid: true}
		}
		moved, err = q.TransitionTask(ctx, bookdb.TransitionTaskParams{
			ID: cur.ID, FromState: cur.State, Version: cur.Version, HeadSha: cur.HeadSha,
			ToState: string(out.State), NewHeadSha: head, NewReturns: int32(out.Returns), //nolint:gosec // returns is bounded by MaxReturns
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return fmt.Errorf("flow: transition task %d: %w", cur.ID, err)
		}
		if out.State == Canceled {
			if _, err := q.CancelTaskJobs(ctx, cur.ID); err != nil {
				return fmt.Errorf("flow: cancel jobs of task %d: %w", cur.ID, err)
			}
		}
		payload, err := json.Marshal(map[string]any{"from": cur.State, "to": out.State, "event": mv.Event, "version": moved.Version})
		if err != nil {
			return fmt.Errorf("flow: %w", err)
		}
		if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
			Tenant: cur.Tenant, TaskID: pgtype.Int8{Int64: cur.ID, Valid: true}, Kind: "task.moved",
			Actor: mv.Actor, Payload: payload, TraceID: pgtype.Text{String: mv.TraceID, Valid: mv.TraceID != ""},
		}); err != nil {
			return fmt.Errorf("flow: record event of task %d: %w", cur.ID, err)
		}
		return nil
	})
	return moved, err
}

// budgetSpent reports whether the task has spent its budget. A task without a budget row has
// no limit.
func budgetSpent(ctx context.Context, q *bookdb.Queries, id int64) (bool, error) {
	b, err := q.GetBudget(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("flow: budget of task %d: %w", id, err)
	}
	return Exhausted(b), nil
}

// Exhausted reports whether iterations or tokens are used up (the hours limit is checked by
// the clock of the running job).
func Exhausted(b bookdb.BookBudget) bool {
	return (b.MaxIterations > 0 && b.UsedIterations >= b.MaxIterations) ||
		(b.MaxTokens > 0 && b.UsedTokens >= b.MaxTokens)
}
