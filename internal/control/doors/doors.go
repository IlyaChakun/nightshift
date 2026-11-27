package doors

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// ErrDoor means a door command cannot be carried out: no such task, not waiting for that
// answer, or the tenant has no owner chat id to act as.
var ErrDoor = errors.New("door")

// Doors are the terminal and MCP entry points of a tenant that works without a chat app (ADR 0006).
// Each call acts as the tenant's owner and goes through the same checks as a chat event; the
// source ("cli", "mcp", "mcp-remote") is recorded with every decision.
type Doors struct {
	Pool   *pgxpool.Pool
	Outbox *outbox.Outbox
	Mover  *flow.Mover
}

// TaskLine is one open task of the status list.
type TaskLine struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

// Pending is a card that waits for the owner's answer.
type Pending struct {
	TaskID  int64            `json:"task_id"`
	Title   string           `json:"title"`
	Version int              `json:"version"`
	Kind    chatapi.CardKind `json:"kind"`
	Text    string           `json:"text"`
	Buttons []chatapi.Button `json:"buttons,omitempty"`
}

// openStates are the states of a task that is not finished.
var openStates = append(dispatch.ActiveStates(),
	string(flow.Spec), string(flow.DesignReview), string(flow.Escalated), string(flow.Blocked))

// Status lists the tenant's open tasks.
func (d *Doors) Status(ctx context.Context, t config.Tenant) ([]TaskLine, error) {
	tasks, err := bookdb.New(d.Pool).ListActiveTasks(ctx, bookdb.ListActiveTasksParams{Tenant: t.Name, States: openStates})
	if err != nil {
		return nil, fmt.Errorf("doors: %w", err)
	}
	out := make([]TaskLine, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, TaskLine{ID: task.ID, Title: task.Title, State: task.State})
	}
	return out, nil
}

// Inbox lists the cards waiting for an answer: the newest card of a task, when the owner has
// not written since and the card asks something (a question, a decision, buttons).
func (d *Doors) Inbox(ctx context.Context, t config.Tenant) ([]Pending, error) {
	q := bookdb.New(d.Pool)
	tasks, err := q.ListActiveTasks(ctx, bookdb.ListActiveTasksParams{Tenant: t.Name, States: openStates})
	if err != nil {
		return nil, fmt.Errorf("doors: %w", err)
	}
	var out []Pending
	for _, task := range tasks {
		events, err := q.ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
		if err != nil {
			return nil, fmt.Errorf("doors: %w", err)
		}
		if p, ok := pendingCard(task, events); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func pendingCard(task bookdb.BookTask, events []bookdb.BookEvent) (Pending, bool) {
	for _, e := range slices.Backward(events) {
		switch e.Kind {
		case "owner_message":
			return Pending{}, false
		case chatapi.InboxEventCard:
			var c chatapi.Card
			if json.Unmarshal(e.Payload, &c) != nil {
				return Pending{}, false
			}
			asks := len(c.Buttons) > 0 || c.Kind == chatapi.CardQuestion || c.Kind == chatapi.CardDecisionNeeded
			if !asks {
				return Pending{}, false
			}
			return Pending{TaskID: task.ID, Title: task.Title, Version: c.Version, Kind: c.Kind, Text: c.Text, Buttons: c.Buttons}, true
		}
	}
	return Pending{}, false
}

// AddIdea starts a task from an idea in the tenant's inbox thread; Lead takes it from there.
func (d *Doors) AddIdea(ctx context.Context, t config.Tenant, repo, text, source string) (bookdb.BookTask, error) {
	var none bookdb.BookTask
	owner, err := ownerOf(t)
	if err != nil {
		return none, err
	}
	if len(t.Repos) == 0 {
		return none, fmt.Errorf("%w: tenant %s has no repos", ErrDoor, t.Name)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return none, fmt.Errorf("%w: the idea is empty", ErrDoor)
	}
	if repo == "" {
		repo = config.RepoName(t.Repos[0])
	}
	// Only a tenant's own repository: its credential and review rules apply to nothing else.
	if !slices.ContainsFunc(t.Repos, func(r string) bool { return config.RepoName(r) == repo }) {
		return none, fmt.Errorf("%w: tenant %s has no repository %q", ErrDoor, t.Name, repo)
	}
	var token [8]byte
	if _, err := rand.Read(token[:]); err != nil {
		return none, fmt.Errorf("doors: %w", err)
	}
	masked := d.Outbox.Mask.String(text)
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return none, fmt.Errorf("doors: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := bookdb.New(tx)
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{
		Tenant: t.Name, Repo: repo, Title: dispatch.Title(masked),
		ThreadRef: pgtype.Text{String: chatapi.InboxChannel + "/" + hex.EncodeToString(token[:]), Valid: true},
	})
	if err != nil {
		return none, fmt.Errorf("doors: %w", err)
	}
	payload, err := json.Marshal(map[string]string{"text": masked, "source": source})
	if err != nil {
		return none, fmt.Errorf("doors: %w", err)
	}
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: t.Name, TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: "task_created", Actor: source + ":" + owner, Payload: payload,
	}); err != nil {
		return none, fmt.Errorf("doors: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return none, fmt.Errorf("doors: %w", err)
	}
	return task, nil
}

// Reply answers the task's thread, for example Lead's question.
func (d *Doors) Reply(ctx context.Context, t config.Tenant, taskID int64, text, source string) error {
	owner, err := ownerOf(t)
	if err != nil {
		return err
	}
	q := bookdb.New(d.Pool)
	task, err := openTask(ctx, q, t, taskID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{"text": d.Outbox.Mask.String(text), "source": source})
	if err != nil {
		return fmt.Errorf("doors: %w", err)
	}
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: t.Name, TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: "owner_message", Actor: source + ":" + owner, Payload: payload,
	}); err != nil {
		return fmt.Errorf("doors: %w", err)
	}
	return nil
}

// Approve presses Approve on the task's current spec card.
func (d *Doors) Approve(ctx context.Context, t config.Tenant, taskID int64, source string) error {
	return d.decide(ctx, t, taskID, "approve", source)
}

// Cancel ends a task that should not run any more, whatever it waits for: a spec nobody wants,
// a pull request that was handled elsewhere, a task stuck in a state nothing watches. It acts as
// the tenant's owner, records the source, and stops the task's jobs. A released task cannot be
// canceled: only a rollback undoes it.
func (d *Doors) Cancel(ctx context.Context, t config.Tenant, taskID int64, source string) error {
	// A door is the owner's own terminal: a tenant without a chat id still has an owner.
	actor := source
	if owner, err := ownerOf(t); err == nil {
		actor = owner + " via " + source
	}
	task, err := openTask(ctx, bookdb.New(d.Pool), t, taskID)
	if err != nil {
		return err
	}
	if _, err := d.Mover.Apply(ctx, flow.Move{
		TaskID: taskID, Event: flow.Cancel, Actor: actor, Profile: flow.ProfileFor(t.Governance), HeadSHA: task.HeadSha.String,
	}); err != nil {
		if errors.Is(err, flow.ErrNotAllowed) {
			return fmt.Errorf("%w: task %d is in %s and cannot be canceled", ErrDoor, taskID, task.State)
		}
		return err
	}
	return nil
}

func (d *Doors) decide(ctx context.Context, t config.Tenant, taskID int64, action, source string) error {
	owner, err := ownerOf(t)
	if err != nil {
		return err
	}
	task, err := openTask(ctx, bookdb.New(d.Pool), t, taskID)
	if err != nil {
		return err
	}
	if s := flow.State(task.State); s != flow.Spec && s != flow.DesignReview {
		return fmt.Errorf("%w: task %d is in %s, nothing waits for approval", ErrDoor, taskID, task.State)
	}
	in := &Inbox{Pool: d.Pool, Outbox: d.Outbox, Mover: d.Mover}
	return in.Handle(ctx, t, chatapi.ChatEvent{
		Kind: chatapi.ChatButton, User: owner, Action: action, TaskID: strconv.FormatInt(taskID, 10),
		CardVersion: int(task.Version), Source: source,
	})
}

// ownerOf is the chat id the doors act as. It is the one the tenant names for its owner, so
// the checks are the same as for chat.
func ownerOf(t config.Tenant) (string, error) {
	if t.People.Owner.Chat == "" {
		return "", fmt.Errorf("%w: tenant %s has no people.owner.chat to act as", ErrDoor, t.Name)
	}
	return t.People.Owner.Chat, nil
}

func openTask(ctx context.Context, q *bookdb.Queries, t config.Tenant, id int64) (bookdb.BookTask, error) {
	task, err := q.GetTask(ctx, id)
	if err != nil || task.Tenant != t.Name {
		return task, fmt.Errorf("%w: tenant %s has no task %d", ErrDoor, t.Name, id)
	}
	return task, nil
}
