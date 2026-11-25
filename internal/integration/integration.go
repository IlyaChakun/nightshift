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

// Reactor is implemented by chat adapters that can put an emoji reaction on a message.
// It is separate from Chat so an adapter without reactions (github-only) needs no stub.
type Reactor interface {
	React(ctx context.Context, msg MessageRef, emoji string) error
}

// PullRequests is the code host as the factory uses it (design sections 7 and 11): open a
// draft PR from a branch nightshift pushed, hand it to people, read CI and reviews back, and
// publish the `nightshift/human-review` status. Merging is added with the merge train.
type PullRequests interface {
	OpenPR(ctx context.Context, pr OpenPR) (PRRef, error)
	PRState(ctx context.Context, ref PRRef) (PRState, error)
	MarkReady(ctx context.Context, ref PRRef) error
	// Merge squashes the pull request when its head is still ref.HeadSHA and returns the
	// merge commit; a moved head is an error, never a merge of unseen code.
	Merge(ctx context.Context, ref PRRef) (string, error)
	RequestReviewers(ctx context.Context, ref PRRef, users []string) error
	AddLabels(ctx context.Context, ref PRRef, labels []string) error
	SetStatus(ctx context.Context, repo, sha string, st CommitStatus) error
	// CI summarizes the checks and statuses of a commit, without the factory's own
	// `nightshift/` statuses, so the factory does not wait for itself.
	CI(ctx context.Context, repo, sha string) (CIResult, error)
	Reviews(ctx context.Context, ref PRRef) ([]Review, error)
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
	// Source is the door the event came through: empty or "chat" for the chat app, "cli", "mcp"
	// or "mcp-remote" for the inbox doors (ADR 0006). It is recorded with the decision.
	Source string    `json:"source,omitempty"`
	At     time.Time `json:"at"`
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

// OpenPR is a request for a pull request. Head is a branch nightshift pushed.
type OpenPR struct {
	Repo  string `json:"repo"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Draft bool   `json:"draft"`
}

// PRState is what the factory needs to know about a pull request.
type PRState struct {
	Ref    PRRef  `json:"ref"`
	State  string `json:"state"` // open, closed or merged
	Draft  bool   `json:"draft"`
	URL    string `json:"url"`
	Merged bool   `json:"merged"`
}

// CommitStatus is a status on a commit; State is pending, success or failure.
type CommitStatus struct {
	Context     string `json:"context"`
	State       string `json:"state"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

// CIState is the summary of the checks of a commit.
type CIState string

// CI summaries. Pending means something still runs or nothing has reported yet.
const (
	CIPending CIState = "pending"
	CISuccess CIState = "success"
	CIFailure CIState = "failure"
)

// CIResult is the CI summary and the names of the failing checks.
type CIResult struct {
	State   CIState  `json:"state"`
	Failing []string `json:"failing,omitempty"`
}

// Review is one review of a pull request at the commit it was made on.
type Review struct {
	User      string `json:"user"`
	State     string `json:"state"` // approved, changes_requested, commented or dismissed
	CommitSHA string `json:"commit_sha"`
}
