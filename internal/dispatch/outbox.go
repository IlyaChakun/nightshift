package dispatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

// Message is one outbox entry on its way to an adapter.
type Message struct {
	ID       int64
	Tenant   string
	TaskID   int64 // 0 when the message belongs to no task
	Target   string
	Payload  []byte // JSON, already masked
	Attempts int32
}

// Sender delivers messages of one target (chat, tracker). It must be idempotent on
// Message.ID: a crash after the send and before the commit delivers the message again.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// ErrPermanent marks a send that will never succeed (a deleted channel, a 4xx other than 429);
// the message fails at once instead of retrying.
var ErrPermanent = errors.New("permanent delivery failure")

// MaxAttempts is how often a message is tried before it is marked failed.
const MaxAttempts = 10

// Backoff returns the wait before the next attempt: 5s, 10s, 20s ... capped at one hour.
func Backoff(attempts int32) time.Duration {
	const first, ceiling = 5 * time.Second, time.Hour
	d := first
	for i := int32(1); i < attempts && d < ceiling; i++ {
		d *= 2
	}
	return min(d, ceiling)
}

// Outbox writes and delivers side effects. Masking happens on the way in and again
// on the way out, so a secret that slipped into a payload never reaches an adapter.
type Outbox struct {
	Pool    *pgxpool.Pool
	Senders map[string]Sender
	Mask    *Masker
	Log     *slog.Logger
	Now     func() time.Time
	Batch   int32
}

// settleTimeout bounds the database writes that record a delivery after shutdown began.
const settleTimeout = 5 * time.Second

// Enqueue stores a message once per idempotency key; a repeat is not an error.
func (o *Outbox) Enqueue(ctx context.Context, q *bookdb.Queries, tenant string, taskID int64, target, key string, payload []byte) error {
	masked, err := o.Mask.JSON(payload)
	if err != nil {
		return err
	}
	_, err = q.EnqueueOutbox(ctx, bookdb.EnqueueOutboxParams{
		Tenant: tenant, TaskID: pgtype.Int8{Int64: taskID, Valid: taskID != 0},
		Target: target, Payload: masked, IdempotencyKey: key,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("outbox: enqueue %s: %w", key, err)
	}
	return nil
}

// Flush delivers the due messages of one batch and returns how many were sent.
func (o *Outbox) Flush(ctx context.Context) (int, error) {
	tx, err := o.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("outbox: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := bookdb.New(tx)
	batch := o.Batch
	if batch == 0 {
		batch = 20
	}
	rows, err := q.ClaimOutbox(ctx, batch)
	if err != nil {
		return 0, fmt.Errorf("outbox: claim: %w", err)
	}
	sent := 0
	for _, row := range rows {
		ok, err := o.deliver(ctx, q, row)
		if err != nil {
			return sent, err
		}
		if ok {
			sent++
		}
	}
	// A message that was sent must be marked even when shutdown cancels ctx now; otherwise
	// the next process sends it again.
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
	defer cancel()
	if err := tx.Commit(commitCtx); err != nil {
		return sent, fmt.Errorf("outbox: commit: %w", err)
	}
	return sent, nil
}

func (o *Outbox) deliver(ctx context.Context, q *bookdb.Queries, row bookdb.BookOutbox) (bool, error) {
	payload, err := o.Mask.JSON(row.Payload)
	if err == nil {
		sender, found := o.Senders[row.Target]
		if !found {
			err = fmt.Errorf("%w: no sender for target %q", ErrPermanent, row.Target)
		} else {
			err = sender.Send(ctx, Message{
				ID: row.ID, Tenant: row.Tenant, TaskID: row.TaskID.Int64, Target: row.Target,
				Payload: payload, Attempts: row.Attempts,
			})
		}
	}
	// Bookkeeping after a send must outlive a canceled ctx, see Flush.
	settle, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
	defer cancel()
	if err == nil {
		return true, wrap(q.MarkOutboxSent(settle, row.ID))
	}
	reason := pgtype.Text{String: o.Mask.String(err.Error()), Valid: true}
	attempts := row.Attempts + 1
	if errors.Is(err, ErrPermanent) || attempts >= MaxAttempts {
		o.log().WarnContext(ctx, "outbox message failed", "id", row.ID, "target", row.Target, "attempts", attempts, "error", reason.String)
		return false, wrap(q.MarkOutboxFailed(settle, bookdb.MarkOutboxFailedParams{ID: row.ID, LastError: reason}))
	}
	next := pgtype.Timestamptz{Time: o.now().Add(Backoff(attempts)), Valid: true}
	return false, wrap(q.MarkOutboxRetry(settle, bookdb.MarkOutboxRetryParams{ID: row.ID, LastError: reason, NextAttemptAt: next}))
}

func (o *Outbox) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *Outbox) log() *slog.Logger {
	if o.Log != nil {
		return o.Log
	}
	return slog.Default()
}

func wrap(err error) error {
	if err != nil {
		return fmt.Errorf("outbox: update: %w", err)
	}
	return nil
}
