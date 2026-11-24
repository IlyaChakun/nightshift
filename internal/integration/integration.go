// Package integration holds the interfaces the core uses to reach external
// systems. The core knows only these; concrete services live in
// internal/adapters and are chosen by name from tenant.yaml (design section 21).
package integration

import (
	"context"
	"time"
)

// Tracker projects tasks into an issue tracker and reads back the only two
// things the factory accepts from it: cancel and priority.
type Tracker interface {
	Upsert(ctx context.Context, t TaskView) (ExternalRef, error)
	Poll(ctx context.Context, since time.Time) ([]TrackerEvent, error)
}

// DocsTarget publishes approved documents.
type DocsTarget interface {
	Publish(ctx context.Context, d Doc) (ExternalRef, error)
}

// Chat posts cards and streams owner events. A chat adapter without its own
// app may post cards as pull request comments.
type Chat interface {
	Post(ctx context.Context, c Card) (MessageRef, error)
	Events(ctx context.Context) (<-chan ChatEvent, error)
}

// ExternalChecks reads findings from checks the project already runs, so the
// factory does not duplicate them.
type ExternalChecks interface {
	Findings(ctx context.Context, pr PRRef) ([]Finding, error)
}

// ExternalRef points to an object in an external system.
type ExternalRef struct {
	System string `json:"system"` // adapter name from tenant.yaml
	ID     string `json:"id"`
	URL    string `json:"url"`
}

// TaskView is what the tracker shows about a task.
type TaskView struct {
	Tenant    string `json:"tenant"`
	TaskID    string `json:"task_id"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Priority  string `json:"priority"` // P0..P3
	ThreadURL string `json:"thread_url,omitempty"`
}

// TrackerEventKind is what the factory accepts from a tracker.
type TrackerEventKind string

// Tracker event kinds.
const (
	TrackerCanceled        TrackerEventKind = "canceled"
	TrackerPriorityChanged TrackerEventKind = "priority_changed"
)

// TrackerEvent is a change made in the tracker.
type TrackerEvent struct {
	Ref      ExternalRef      `json:"ref"`
	Kind     TrackerEventKind `json:"kind"`
	Priority string           `json:"priority,omitempty"`
	Actor    string           `json:"actor"` // checked against owner and approvers
	At       time.Time        `json:"at"`
}

// Doc is a document to publish.
type Doc struct {
	Tenant string `json:"tenant"`
	Path   string `json:"path"` // path in the repository, for example docs/adr/0001-x.md
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// CardKind is one of the four message types (design section 7).
type CardKind string

// Card kinds.
const (
	CardQuestion       CardKind = "question"
	CardDecisionNeeded CardKind = "decision_needed"
	CardReady          CardKind = "ready_for_acceptance"
	CardBlocker        CardKind = "blocker_or_summary"
)

// Card is one message with optional buttons. Version rejects clicks on stale
// cards.
type Card struct {
	Tenant  string     `json:"tenant"`
	TaskID  string     `json:"task_id"`
	Version int        `json:"version"`
	Kind    CardKind   `json:"kind"`
	Text    string     `json:"text"`
	Buttons []Button   `json:"buttons,omitempty"`
	Thread  MessageRef `json:"thread"`
}

// Button is an action on a card.
type Button struct {
	Action string `json:"action"` // approve, review, edit, reject, merge, rollback
	Label  string `json:"label"`
}

// MessageRef points to a chat message or thread.
type MessageRef struct {
	Channel string `json:"channel"`
	ID      string `json:"id"`
}

// ChatEventKind classifies incoming chat events.
type ChatEventKind string

// Chat event kinds.
const (
	ChatMessage  ChatEventKind = "message"
	ChatButton   ChatEventKind = "button"
	ChatCommand  ChatEventKind = "command"
	ChatReaction ChatEventKind = "reaction"
	ChatHomeOpen ChatEventKind = "home_opened"
)

// ChatEvent is something a person did in chat.
type ChatEvent struct {
	Kind        ChatEventKind `json:"kind"`
	User        string        `json:"user"` // checked against owner and approvers
	Thread      MessageRef    `json:"thread"`
	MessageID   string        `json:"message_id,omitempty"` // unique per message; replies are keyed by it
	Text        string        `json:"text,omitempty"`
	Action      string        `json:"action,omitempty"`
	TaskID      string        `json:"task_id,omitempty"`
	CardVersion int           `json:"card_version,omitempty"`
	At          time.Time     `json:"at"`
}

// PRRef points to a pull request at a given head.
type PRRef struct {
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	HeadSHA string `json:"head_sha"`
}

// Finding mirrors schemas/finding.json.
type Finding struct {
	ID            string    `json:"id,omitempty"`
	RuleID        string    `json:"rule_id"`
	Category      string    `json:"category"`
	Severity      string    `json:"severity"`
	LayerCaught   string    `json:"layer_caught"`
	LayerExpected string    `json:"layer_expected,omitempty"`
	Message       string    `json:"message"`
	Location      *Location `json:"location,omitempty"`
	Evidence      []string  `json:"evidence,omitempty"`
	Fingerprint   string    `json:"fingerprint,omitempty"`
}

// Location is where a finding points.
type Location struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}
