// Package integration holds the interfaces the core uses to reach external
// systems. The core knows only these; concrete services live in
// internal/adapters and are chosen by name from tenant.yaml (design section 21).
package integration

import (
	"context"
	"errors"
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
	// LineComments renders the comments one review left on lines of the diff. Ask it only for
	// a review of a person who may decide: anyone can review a public repository.
	LineComments(ctx context.Context, ref PRRef, review int64) (string, error)
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

// TrackerEvent is a change made in the tracker. An adapter returns the events after `since` in
// time order, and names the actor by an id it has verified (the tenant's chat or GitHub id), not
// by a display name.
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
	// MergeSHA is the commit the merge made on the base branch, when the pull request is merged.
	// A release deploys it, never the head of the branch.
	MergeSHA string `json:"merge_sha,omitempty"`
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
	// Detail is what the failing checks say about themselves (their summary and the messages
	// they put on lines), bounded; empty when the code host gave nothing. Full job logs are not
	// read: they come from a second host.
	Detail string `json:"detail,omitempty"`
}

// Review is one review of a pull request at the commit it was made on.
type Review struct {
	User      string `json:"user"`
	State     string `json:"state"` // approved, changes_requested, commented or dismissed
	CommitSHA string `json:"commit_sha"`
	ID        int64  `json:"id"`
	Body      string `json:"body"` // the reviewer's summary
}

// MergeLog lists what was merged into a repository, for the morning digest (ADR 0013): the
// owner reads every merge of the night, whoever made it, with its reason and a way back. An
// error may come with a partial list; the caller shows both.
type MergeLog interface {
	MergedSince(ctx context.Context, repo string, since time.Time) ([]MergedPR, error)
}

// MergedPR is one merged pull request.
type MergedPR struct {
	Number   int       `json:"number"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	MergedBy string    `json:"merged_by"`
	MergedAt time.Time `json:"merged_at"`
	Labels   []string  `json:"labels,omitempty"`
	// Reason is the "What and why" paragraph of the description, cut short.
	Reason string `json:"reason,omitempty"`
	// Reviewed is true when the description records an independent review ("Reviewed by").
	Reviewed bool `json:"reviewed"`
}

// PlanLog reads how far a repository's own plan has come, for the morning digest. A
// repository without a plan returns an empty list and no error.
type PlanLog interface {
	PlanProgress(ctx context.Context, repo string) ([]PlanProgress, error)
}

// PlanProgress is one part of the plan, a file or a folder: its name (the heading it opens with,
// else the name of the file or folder) and its checklist items, done and in all.
type PlanProgress struct {
	Name  string `json:"name"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// Deployer is a deploy target as the conductor uses it (design section 17, "Deploy targets",
// ADR 0014): a preview or production environment on one platform. Every operation is
// idempotent and keyed by tenant, task and step, so after a restart the conductor asks Status
// and continues. An adapter that cannot do an operation returns ErrUnsupported.
type Deployer interface {
	// Up brings an environment up at a revision and returns where it answers.
	Up(ctx context.Context, e EnvRef, rev Revision) (Endpoint, error)
	// Down removes an environment; an environment that is already absent is not an error.
	Down(ctx context.Context, e EnvRef) error
	// Release makes rev the production revision and returns the one it replaced. It must be safe
	// to call again for the same rev after a crash and then return the same Previous: the
	// conductor's journal repeats a Release whose answer it did not record. When rev was started
	// but did not become healthy, it returns the Released it knows (Previous set) together with
	// an error that wraps ErrRolloutFailed, so the conductor can roll back.
	Release(ctx context.Context, rev Revision) (Released, error)
	// Smoke checks an environment that is up: healthchecks first, then the project's own check.
	Smoke(ctx context.Context, e EnvRef) (SmokeResult, error)
	// Rollback makes to the production revision again.
	Rollback(ctx context.Context, to Revision) error
	Status(ctx context.Context, e EnvRef) (EnvStatus, error)
}

// ErrUnsupported is returned by an adapter for an operation its platform does not have.
var ErrUnsupported = errors.New("deploy operation is not supported by this adapter")

// ErrRolloutFailed means the new revision was started and did not come up healthy: a defect of
// the revision, not of the platform. The previous revision is still the way back.
var ErrRolloutFailed = errors.New("the new revision did not become healthy")

// ErrRefused means the platform refused the change itself, for example a recipe that breaks a
// rule of the adapter: a defect of the code under test, not of the platform, so the task goes
// back to Builder and nothing is retried.
var ErrRefused = errors.New("the deploy target refused the change")

// Revision is a commit: of the task branch for a preview, of main for a release.
type Revision string

// EnvRef names one environment. Task is empty for production.
type EnvRef struct {
	Tenant string `json:"tenant"`
	Task   string `json:"task,omitempty"`
}

// Endpoint is where an environment answers.
type Endpoint struct {
	URL string `json:"url"`
}

// Released reports a release: the revision that was live before it, for rollback.
type Released struct {
	Revision Revision `json:"revision"`
	Previous Revision `json:"previous,omitempty"`
}

// SmokeResult is the outcome of a smoke check; Detail is the short reason when it failed.
type SmokeResult struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// EnvState is the health of an environment.
type EnvState string

// The states of an environment.
const (
	EnvAbsent    EnvState = "absent"
	EnvStarting  EnvState = "starting"
	EnvHealthy   EnvState = "healthy"
	EnvUnhealthy EnvState = "unhealthy"
)

// EnvStatus is what Status reports.
type EnvStatus struct {
	State    EnvState `json:"state"`
	Revision Revision `json:"revision,omitempty"`
	// Previous is the revision this one replaced, when the platform remembers it.
	Previous Revision `json:"previous,omitempty"`
}

// RepoHealth reads what a person would otherwise have to open the code host to see: open
// code-scanning alerts, open pull requests and the state of the default branch. The morning
// digest shows it (docs/design.md, section 10, factory health).
type RepoHealth interface {
	OpenAlerts(ctx context.Context, repo string) ([]Alert, error)
	OpenPullRequests(ctx context.Context, repo string) ([]PRActivity, error)
	// DefaultBranchFailures names the failing checks of the latest commit of the default branch.
	DefaultBranchFailures(ctx context.Context, repo string) ([]string, error)
}

// Alert is one open code-scanning alert.
type Alert struct {
	Repo     string    `json:"repo"` // owner/name; set by the reader of several repositories
	Number   int       `json:"number"`
	Rule     string    `json:"rule"`
	Severity string    `json:"severity"` // critical, high, medium, low, or the rule level when none is set
	Path     string    `json:"path"`
	URL      string    `json:"url"`
	Created  time.Time `json:"created"`
}

// PRActivity is one open pull request.
type PRActivity struct {
	Repo    string    `json:"repo"` // owner/name; set by the reader of several repositories
	Number  int       `json:"number"`
	Title   string    `json:"title"`
	URL     string    `json:"url"`
	Draft   bool      `json:"draft"`
	Updated time.Time `json:"updated"`
}
