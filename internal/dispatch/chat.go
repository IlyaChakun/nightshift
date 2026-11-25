package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
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
	Mover  *flow.Mover // applies the owner's button clicks; nil ignores them
	Log    *slog.Logger
}

// Handle records one chat event. A new thread creates a task; a message in a known
// thread is noted on its task. Each message gets one reply, keyed by its id.
func (in *Inbox) Handle(ctx context.Context, t config.Tenant, ev integration.ChatEvent) error {
	if !policy.CanDecideInChat(t.People, ev.User) {
		return nil
	}
	if ev.Kind == integration.ChatButton {
		return in.button(ctx, t, ev)
	}
	if ev.Kind != integration.ChatMessage {
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
		Tenant: t.Name, Repo: repoName(t.Repos[0]), Title: title(in.Outbox.Mask.String(ev.Text)), ThreadRef: ref,
	})
	if err != nil {
		return task, "", fmt.Errorf("inbox: create task: %w", err)
	}
	return task, "task_created", nil
}

func (in *Inbox) reply(ctx context.Context, q *bookdb.Queries, t config.Tenant, task bookdb.BookTask, kind string, ev integration.ChatEvent) error {
	text := replyText(kind == "task_created", task.ID, ev.Text)
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

// replyText is the bot's answer to an owner message: short, in the language of the
// message, no echo of the task text, and it says what happens next. A first message
// opens a task; later messages in its thread add to it.
func replyText(created bool, taskID int64, msg string) string {
	ru := isRussian(msg)
	switch {
	case created && ru:
		return fmt.Sprintf("Принял, это задача #%d. Lead подготовит спеку и пришлёт её сюда на согласование.", taskID)
	case created:
		return fmt.Sprintf("Got it, this is task #%d. Lead will draft a spec and post it here for your OK.", taskID)
	case ru:
		return fmt.Sprintf("Учёл в задаче #%d.", taskID)
	}
	return fmt.Sprintf("Added to task #%d.", taskID)
}

// isRussian reports whether the message has Cyrillic letters. Chat language is "auto"
// (design section 17): a reply follows the message, English is the default, and a
// Russian message with English technical terms still gets a Russian reply.
func isRussian(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) >= 0
}

func (in *Inbox) log() *slog.Logger {
	if in.Log != nil {
		return in.Log
	}
	return slog.Default()
}

// repoName drops the host from a repository in tenant.yaml (host/owner/name): the book, git
// mirrors and the pull request adapter name a repository owner/name.
func repoName(repo string) string {
	if _, name, ok := strings.Cut(repo, "/"); ok && strings.Contains(name, "/") {
		return name
	}
	return repo
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
	card.Delivery = m.ID
	chat, ok := s[m.Tenant]
	if !ok {
		return fmt.Errorf("%w: no chat for tenant %q", ErrPermanent, m.Tenant)
	}
	if _, err := chat.Post(ctx, card); err != nil {
		if permanentChatError(err) {
			return fmt.Errorf("%w: chat post: %w", ErrPermanent, err)
		}
		return fmt.Errorf("chat post: %w", err)
	}
	return nil
}

// permanentChatCodes are Slack errors that a retry cannot fix: the card or the channel is wrong.
// not_in_channel is left out: inviting the bot fixes it while the message still retries.
var permanentChatCodes = []string{
	"invalid_blocks", "invalid_blocks_format", "msg_too_long", "channel_not_found", "is_archived",
}

func permanentChatError(err error) bool {
	msg := err.Error()
	return slices.ContainsFunc(permanentChatCodes, func(c string) bool { return strings.Contains(msg, c) })
}

// buttonEvents maps a spec card's buttons to flow events.
var buttonEvents = map[string]flow.Event{"approve": flow.Approve, "edit": flow.Edit, "reject": flow.Reject}

// button applies the owner's click on a spec card. A click on a card of an older version, or
// on a task that is not waiting for the owner, changes nothing.
func (in *Inbox) button(ctx context.Context, t config.Tenant, ev integration.ChatEvent) error {
	e, ok := buttonEvents[ev.Action]
	if !ok || in.Mover == nil {
		return nil
	}
	id, err := strconv.ParseInt(ev.TaskID, 10, 64)
	if err != nil {
		return nil //nolint:nilerr // a card without a task id is not ours
	}
	q := bookdb.New(in.Pool)
	task, err := q.GetTask(ctx, id)
	if err != nil || task.Tenant != t.Name || (flow.State(task.State) != flow.Spec && flow.State(task.State) != flow.DesignReview) {
		return nil //nolint:nilerr // a stale or foreign click is not an error
	}
	if ev.CardVersion != 0 && ev.CardVersion != int(task.Version) {
		return nil
	}
	profile := flow.Solo
	if t.Governance == "team" {
		profile = flow.Team
	}
	if _, err := in.Mover.Apply(ctx, flow.Move{TaskID: task.ID, Event: e, Actor: actorOf(ev), Profile: profile}); err != nil {
		if errors.Is(err, flow.ErrConflict) {
			return nil
		}
		return fmt.Errorf("inbox: %w", err)
	}
	ref, err := threadOf(task)
	if err != nil {
		return nil //nolint:nilerr // no thread, nowhere to answer
	}
	ru := isRussian(task.Title)
	text := map[string][2]string{
		"approve": {"Approved. Builder starts now.", "Принято. Builder начинает работу."},
		"edit":    {"What should change?", "Что изменить?"},
		"reject":  {"Rejected. The task is closed.", "Отклонено. Задача закрыта."},
	}[ev.Action]
	msg := text[0]
	if ru {
		msg = text[1]
	}
	card, err := json.Marshal(integration.Card{Tenant: t.Name, TaskID: ev.TaskID, Version: int(task.Version) + 1, Kind: integration.CardBlocker, Text: msg, Thread: ref})
	if err != nil {
		return fmt.Errorf("inbox: %w", err)
	}
	return in.Outbox.Enqueue(ctx, q, t.Name, task.ID, ChatTarget, "button:"+ev.TaskID+":"+ev.Action+":"+strconv.Itoa(int(task.Version)), card)
}

// actorOf names who did it and through which door: "chat:U1" from the chat app, "cli:owner" from the
// terminal.
func actorOf(ev integration.ChatEvent) string {
	if ev.Source == "" || ev.Source == "chat" {
		return "chat:" + ev.User
	}
	return ev.Source + ":" + ev.User
}
