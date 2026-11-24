package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/policy"
)

// ChatTarget is the outbox target for chat cards.
const ChatTarget = "chat"

// maxTitle bounds a task title taken from a chat message, in runes.
const maxTitle = 120

// Route ties one chat adapter to the tenants that use it.
type Route struct {
	Chat    integration.Chat
	Tenants []config.Tenant
}

// TenantFor returns the tenant that owns a channel. When only one tenant uses the
// adapter, every channel belongs to it.
func (r Route) TenantFor(channel string) (config.Tenant, bool) {
	for _, t := range r.Tenants {
		for _, c := range t.Chat.Channels {
			if c == channel {
				return t, true
			}
		}
	}
	if len(r.Tenants) == 1 {
		return r.Tenants[0], true
	}
	return config.Tenant{}, false
}

// Inbox turns the owner's chat messages into tasks and answers in their thread through
// the outbox. Messages from anyone else are ignored (design section 7).
type Inbox struct {
	Pool   *pgxpool.Pool
	Outbox *Outbox
	Log    *slog.Logger
}

// Handle records one chat event. A new thread creates a task; a message in a known
// thread is noted on its task. Each message gets one reply, keyed by its id.
func (in *Inbox) Handle(ctx context.Context, t config.Tenant, ev integration.ChatEvent) error {
	if ev.Kind != integration.ChatMessage || !policy.CanDecideInChat(t.People, ev.User) {
		return nil
	}
	ctx, span := otel.Tracer("nightshift/dispatch").Start(ctx, "chat.message")
	defer span.End()

	tx, err := in.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inbox: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := bookdb.New(tx)

	task, kind, err := in.taskFor(ctx, q, t, ev)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{"text": in.Outbox.Mask.String(ev.Text)})
	if err != nil {
		return fmt.Errorf("inbox: event payload: %w", err)
	}
	trace := pgtype.Text{}
	if sc := span.SpanContext(); sc.HasTraceID() {
		trace = pgtype.Text{String: sc.TraceID().String(), Valid: true}
	}
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: t.Name, TaskID: pgtype.Int8{Int64: task.ID, Valid: true},
		Kind: kind, Actor: "chat:" + ev.User, Payload: payload, TraceID: trace,
	}); err != nil {
		return fmt.Errorf("inbox: append event: %w", err)
	}
	if err := in.reply(ctx, q, t, task, kind, ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("inbox: commit: %w", err)
	}
	// Deliver now instead of waiting for the periodic flush; a failure is retried there.
	if _, err := in.Outbox.Flush(ctx); err != nil {
		in.log().WarnContext(ctx, "outbox flush after chat message failed", "error", err)
	}
	return nil
}

func (in *Inbox) taskFor(ctx context.Context, q *bookdb.Queries, t config.Tenant, ev integration.ChatEvent) (bookdb.BookTask, string, error) {
	ref := pgtype.Text{String: ev.Thread.Channel + "/" + ev.Thread.ID, Valid: true}
	task, err := q.GetTaskByThread(ctx, bookdb.GetTaskByThreadParams{Tenant: t.Name, ThreadRef: ref})
	if err == nil {
		return task, "owner_message", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return task, "", fmt.Errorf("inbox: find task: %w", err)
	}
	if len(t.Repos) == 0 {
		return task, "", fmt.Errorf("inbox: tenant %s has no repos", t.Name)
	}
	task, err = q.CreateTask(ctx, bookdb.CreateTaskParams{
		Tenant: t.Name, Repo: t.Repos[0], Title: title(in.Outbox.Mask.String(ev.Text)), ThreadRef: ref,
	})
	if err != nil {
		return task, "", fmt.Errorf("inbox: create task: %w", err)
	}
	return task, "task_created", nil
}

func (in *Inbox) reply(ctx context.Context, q *bookdb.Queries, t config.Tenant, task bookdb.BookTask, kind string, ev integration.ChatEvent) error {
	text := fmt.Sprintf("Noted on task #%d.", task.ID)
	if kind == "task_created" {
		text = fmt.Sprintf("Task #%d recorded: %s", task.ID, task.Title)
	}
	card, err := json.Marshal(integration.Card{
		Tenant: t.Name, TaskID: strconv.FormatInt(task.ID, 10), Version: int(task.Version),
		Kind: integration.CardBlocker, Text: text, Thread: ev.Thread,
	})
	if err != nil {
		return fmt.Errorf("inbox: card: %w", err)
	}
	id := ev.MessageID
	if id == "" {
		id = ev.Thread.ID
	}
	key := "chat-reply:" + ev.Thread.Channel + ":" + id
	return in.Outbox.Enqueue(ctx, q, t.Name, task.ID, ChatTarget, key, card)
}

func (in *Inbox) log() *slog.Logger {
	if in.Log != nil {
		return in.Log
	}
	return slog.Default()
}

// title is the first line of a message, cut to maxTitle runes.
func title(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	line = strings.TrimSpace(line)
	if r := []rune(line); len(r) > maxTitle {
		line = string(r[:maxTitle-1]) + "…"
	}
	if line == "" {
		return "(empty message)"
	}
	return line
}

// ChatSender delivers chat cards from the outbox to each tenant's adapter.
type ChatSender map[string]integration.Chat

// Send implements Sender.
func (s ChatSender) Send(ctx context.Context, m Message) error {
	var card integration.Card
	if err := json.Unmarshal(m.Payload, &card); err != nil {
		return fmt.Errorf("%w: bad card: %w", ErrPermanent, err)
	}
	chat, ok := s[m.Tenant]
	if !ok {
		return fmt.Errorf("%w: no chat for tenant %q", ErrPermanent, m.Tenant)
	}
	if _, err := chat.Post(ctx, card); err != nil {
		return fmt.Errorf("chat post: %w", err)
	}
	return nil
}
