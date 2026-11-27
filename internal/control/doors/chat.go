package doors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"

	"github.com/ilyachakun/nightshift/internal/assurance/policy"
	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// Route ties one chat adapter to the tenants that use it.
type Route struct {
	Chat    chatapi.Chat
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
	Outbox *outbox.Outbox
	Mover  *flow.Mover // applies the owner's button clicks; nil ignores them
	Log    *slog.Logger
}

// Handle records one chat event. A new thread creates a task; a message in a known
// thread is noted on its task. Each message gets one reply, keyed by its id.
func (in *Inbox) Handle(ctx context.Context, t config.Tenant, ev chatapi.ChatEvent) error {
	if !policy.CanDecideInChat(t.People, ev.User) {
		return nil
	}
	if ev.Kind == chatapi.ChatButton {
		return in.button(ctx, t, ev)
	}
	if ev.Kind == chatapi.ChatCommand {
		return in.command(ctx, t, ev)
	}
	if ev.Kind != chatapi.ChatMessage {
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

func (in *Inbox) taskFor(ctx context.Context, q *bookdb.Queries, t config.Tenant, ev chatapi.ChatEvent) (bookdb.BookTask, string, error) {
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
		Tenant: t.Name, Repo: config.RepoName(t.Repos[0]), Title: chatapi.Title(in.Outbox.Mask.String(ev.Text)), ThreadRef: ref,
	})
	if err != nil {
		return task, "", fmt.Errorf("inbox: create task: %w", err)
	}
	return task, "task_created", nil
}

func (in *Inbox) reply(ctx context.Context, q *bookdb.Queries, t config.Tenant, task bookdb.BookTask, kind string, ev chatapi.ChatEvent) error {
	text := replyText(kind == "task_created", task.ID, chatapi.WantsRussian(t.Language.Chat, ev.Text))
	card, err := json.Marshal(chatapi.Card{
		Tenant: t.Name, TaskID: strconv.FormatInt(task.ID, 10), Version: int(task.Version),
		Kind: chatapi.CardBlocker, Text: text, Thread: ev.Thread,
	})
	if err != nil {
		return fmt.Errorf("inbox: card: %w", err)
	}
	id := ev.MessageID
	if id == "" {
		id = ev.Thread.ID
	}
	key := "chat-reply:" + ev.Thread.Channel + ":" + id
	return in.Outbox.Enqueue(ctx, q, t.Name, task.ID, outbox.ChatTarget, key, card)
}

// replyText is the bot's answer to an owner message: short, in the language of the
// message, no echo of the task text, and it says what happens next. A first message
// opens a task; later messages in its thread add to it.
func replyText(created bool, taskID int64, ru bool) string {
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

func (in *Inbox) log() *slog.Logger {
	if in.Log != nil {
		return in.Log
	}
	return slog.Default()
}

// buttonEvents maps a spec card's buttons to flow events.
var buttonEvents = map[string]flow.Event{
	"approve": flow.Approve, "edit": flow.Edit, "reject": flow.Reject,
	"proceed": flow.BlockDecided, "cancel_task": flow.Cancel, // the card of a task blocked by a canceled dependency
}

// waitsForOwner says whether a click is valid in the task's state: spec buttons on a spec, the
// two decisions on a blocked task.
func waitsForOwner(state flow.State, action string) bool {
	if action == "proceed" || action == "cancel_task" {
		return state == flow.Blocked
	}
	return state == flow.Spec || state == flow.DesignReview
}

// button applies the owner's click on a spec card. A click on a card of an older version, or
// on a task that is not waiting for the owner, changes nothing.
func (in *Inbox) button(ctx context.Context, t config.Tenant, ev chatapi.ChatEvent) error {
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
	if err != nil || task.Tenant != t.Name || !waitsForOwner(flow.State(task.State), ev.Action) {
		return nil //nolint:nilerr // a stale or foreign click is not an error
	}
	if ev.CardVersion != 0 && ev.CardVersion != int(task.Version) {
		return nil
	}
	profile := flow.Solo
	if t.Governance == "team" {
		profile = flow.Team
	}
	if ev.Action == "proceed" {
		// Going on without the canceled dependencies: they stop counting, or the task would block again.
		if err := dispatch.DropBrokenDeps(ctx, q, task.ID); err != nil {
			return fmt.Errorf("inbox: %w", err)
		}
	}
	// The move is bound to the version read here, so a card replaced in between is not approved.
	scheduled, err := in.applyButton(ctx, flow.Move{TaskID: task.ID, Event: e, Actor: actorOf(ev), Profile: profile, Version: task.Version})
	if err != nil {
		if errors.Is(err, flow.ErrConflict) {
			return nil
		}
		return fmt.Errorf("inbox: %w", err)
	}
	ref, err := dispatch.ThreadOf(task)
	if err != nil {
		return nil //nolint:nilerr // no thread, nowhere to answer
	}
	ru := chatapi.WantsRussian(t.Language.Chat, task.Title)
	text := map[string][2]string{
		"approve":     {"Approved. Builder starts now.", "Принято. Builder начинает работу."},
		"edit":        {"What should change?", "Что изменить?"},
		"reject":      {"Rejected. The task is closed.", "Отклонено. Задача закрыта."},
		"proceed":     {"Going on without the canceled task.", "Продолжаю без отменённой задачи."},
		"cancel_task": {"Canceled.", "Отменено."},
	}[ev.Action]
	if scheduled {
		text = [2]string{"Approved. Tasks are queued with their schedule.", "Принято. Задачи поставлены в очередь по расписанию."}
	}
	msg := text[0]
	if ru {
		msg = text[1]
	}
	card, err := json.Marshal(chatapi.Card{Tenant: t.Name, TaskID: ev.TaskID, Version: int(task.Version) + 1, Kind: chatapi.CardBlocker, Text: msg, Thread: ref})
	if err != nil {
		return fmt.Errorf("inbox: %w", err)
	}
	return in.Outbox.Enqueue(ctx, q, t.Name, task.ID, outbox.ChatTarget, "button:"+ev.TaskID+":"+ev.Action+":"+strconv.Itoa(int(task.Version)), card)
}

// applyButton persists an approved schedule before the scheduler can see the queued tasks.
func (in *Inbox) applyButton(ctx context.Context, move flow.Move) (bool, error) {
	if move.Event != flow.Approve {
		_, err := in.Mover.Apply(ctx, move)
		return false, err
	}
	var scheduled bool
	err := pgx.BeginFunc(ctx, in.Pool, func(tx pgx.Tx) error {
		moved, err := in.Mover.ApplyInTx(ctx, tx, move)
		if err != nil {
			return err
		}
		scheduled, err = dispatch.ExpandApprovedTasks(ctx, bookdb.New(tx), moved)
		return err
	})
	return scheduled, err
}

// actorOf names who did it and through which door: "chat:U1" from the chat app, "cli:owner" from the
// terminal.
func actorOf(ev chatapi.ChatEvent) string {
	if ev.Source == "" || ev.Source == "chat" {
		return "chat:" + ev.User
	}
	return ev.Source + ":" + ev.User
}
