// Package chatapi is chat as the conductor uses it (design section 7): it posts cards and reads
// what people did. The chat app adapter, the inbox door in the book and the twin implement it; concrete
// adapters live in internal/adapters and are chosen by name from tenant.yaml.
package chatapi

import (
	"context"
	"time"
)

// InboxEventCard is the book event kind of a card the inbox door stored (ADR 0006); the doors
// read pending cards back by it.
const InboxEventCard = "inbox.card"

// InboxChannel is the thread channel of every inbox thread.
const InboxChannel = "inbox"

// Chat posts cards and streams owner events. A chat adapter without its own
// app may post cards as pull request comments.
type Chat interface {
	Post(ctx context.Context, c Card) (MessageRef, error)
	Events(ctx context.Context) (<-chan ChatEvent, error)
}

// Reactor is implemented by chat adapters that can put an emoji reaction on a message.
// It is separate from Chat so an adapter without reactions (github-only) needs no stub.
type Reactor interface {
	React(ctx context.Context, msg MessageRef, emoji string) error
}

// CardKind is one of the four message types of Lead (design section 7), or the spec card that the
// conductor posts when a spec waits for the owner's Approve.
type CardKind string

// Card kinds.
const (
	CardQuestion       CardKind = "question"
	CardDecisionNeeded CardKind = "decision_needed"
	CardReady          CardKind = "ready_for_acceptance"
	CardBlocker        CardKind = "blocker_or_summary"
	CardSpec           CardKind = "spec_for_approval"
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
	// Delivery identifies the outbox message that carries the card; the sender sets it, so an
	// adapter can tell a redelivery from a new card with the same text.
	Delivery int64 `json:"delivery,omitempty"`
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
	// Source is the door the event came through: empty or "chat" for the chat app, "cli", "mcp"
	// or "mcp-remote" for the inbox doors (ADR 0006). It is recorded with the decision.
	Source string    `json:"source,omitempty"`
	At     time.Time `json:"at"`
}
